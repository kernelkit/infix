/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * gRPC backend for netd - communicates with FRR's management daemon
 * using the gRPC northbound API. This provides a standard protocol
 * interface for configuration management.
 */

#include <memory>
#include <string>
#include <sstream>

#include <grpcpp/grpcpp.h>
#include <grpcpp/create_channel.h>
#include <grpcpp/security/credentials.h>

#include "grpc/frr-northbound.grpc.pb.h"
#include "grpc/frr-northbound.pb.h"

extern "C" {
#include "netd.h"
#include "route.h"
#include "rip.h"
#include "grpc_backend.h"
}

/* Global gRPC state */
static std::shared_ptr<grpc::Channel> g_channel;
static std::unique_ptr<frr::Northbound::Stub> g_stub;
static uint64_t g_candidate_id = 0;

/* gRPC server address */
#define GRPC_SERVER "127.0.0.1:50051"

/*
 * Helper to check if two routes have the same prefix.
 * Routes with the same prefix should be grouped into one route-list
 * entry with multiple path-list entries.
 */
static bool same_prefix(const struct route *a, const struct route *b)
{
	if (a->family != b->family || a->prefixlen != b->prefixlen)
		return false;

	if (a->family == AF_INET)
		return memcmp(&a->prefix.ip4, &b->prefix.ip4, sizeof(a->prefix.ip4)) == 0;
	else
		return memcmp(&a->prefix.ip6, &b->prefix.ip6, sizeof(a->prefix.ip6)) == 0;
}

/*
 * Build JSON configuration for staticd following FRR's YANG model.
 * Groups routes with the same prefix into a single route-list entry
 * with multiple path-list entries (for ECMP/multipath support).
 */
static std::string build_staticd_json(struct route_head *routes)
{
	struct route *r, *next;
	char buf[INET6_ADDRSTRLEN];
	std::ostringstream json;
	bool first_route = true;

	json << "{"
	     << "\"frr-routing:routing\":{"
	     << "\"control-plane-protocols\":{"
	     << "\"control-plane-protocol\":[{"
	     << "\"type\":\"frr-staticd:staticd\","
	     << "\"name\":\"staticd\","
	     << "\"vrf\":\"default\","
	     << "\"frr-staticd:staticd\":{"
	     << "\"route-list\":[";

	r = TAILQ_FIRST(routes);
	while (r != NULL) {
		const char *afi;

		if (r->family == AF_INET) {
			inet_ntop(AF_INET, &r->prefix.ip4, buf, sizeof(buf));
			afi = "frr-routing:ipv4-unicast";
		} else {
			inet_ntop(AF_INET6, &r->prefix.ip6, buf, sizeof(buf));
			afi = "frr-routing:ipv6-unicast";
		}

		if (!first_route)
			json << ",";
		first_route = false;

		/* Start route-list entry */
		json << "{"
		     << "\"prefix\":\"" << buf << "/" << (int)r->prefixlen << "\","
		     << "\"src-prefix\":\"::/0\","  /* FRR expects IPv6 format */
		     << "\"afi-safi\":\"" << afi << "\","
		     << "\"path-list\":[";

		/* Add all routes with the same prefix as path-list entries */
		bool first_path = true;
		struct route *curr = r;
		while (curr != NULL && same_prefix(r, curr)) {
			if (!first_path)
				json << ",";
			first_path = false;

			/* Start path-list entry */
			json << "{"
			     << "\"table-id\":0,"
			     << "\"distance\":" << (int)curr->distance << ","
			     << "\"frr-nexthops\":{\"nexthop\":[{";

			/* Next-hop */
			switch (curr->nh_type) {
			case NH_ADDR:
				if (curr->family == AF_INET)
					inet_ntop(AF_INET, &curr->gateway.gw4, buf, sizeof(buf));
				else
					inet_ntop(AF_INET6, &curr->gateway.gw6, buf, sizeof(buf));

				json << "\"nh-type\":\"" << (curr->family == AF_INET ? "ip4" : "ip6") << "\","
				     << "\"vrf\":\"default\","
				     << "\"gateway\":\"" << buf << "\","
				     << "\"interface\":\"\"";
				break;

			case NH_IFNAME:
				json << "\"nh-type\":\"ifindex\","
				     << "\"vrf\":\"default\","
				     << "\"gateway\":\"\","
				     << "\"interface\":\"" << curr->ifname << "\"";
				break;

			case NH_BLACKHOLE:
				json << "\"nh-type\":\"blackhole\","
				     << "\"vrf\":\"default\","
				     << "\"gateway\":\"\","
				     << "\"interface\":\"\",";

				switch (curr->bh_type) {
				case BH_DROP:
					json << "\"bh-type\":\"null\"";
					break;
				case BH_REJECT:
					json << "\"bh-type\":\"reject\"";
					break;
				case BH_NULL:
					json << "\"bh-type\":\"unspec\"";
					break;
				}
				break;
			}

			/* Close nexthop object, nexthop array, frr-nexthops, and path-list entry */
			json << "}]}}";

			/* Move to next route */
			curr = TAILQ_NEXT(curr, entries);
		}

		/* Close path-list array and route-list entry */
		json << "]}";

		/* Move r to the next different prefix */
		r = curr;
	}

	/* Close all JSON structures */
	json << "]}}]}}}";

	return json.str();
}

/*
 * Build JSON configuration for RIP following FRR's YANG model
 * RIP uses /frr-ripd:ripd/instance path, not control-plane-protocols
 */
static std::string build_rip_json(struct rip_config *rip_cfg)
{
	std::ostringstream json;
	char buf[INET_ADDRSTRLEN];
	bool first;

	if (!rip_cfg || !rip_cfg->enabled)
		return "";

	json << "{"
	     << "\"frr-ripd:ripd\":{"
	     << "\"instance\":[{"
	     << "\"vrf\":\"default\",";

	/* Default metric */
	if (rip_cfg->default_metric != 1)
		json << "\"default-metric\":" << (int)rip_cfg->default_metric << ",";

	/* Distance */
	if (rip_cfg->distance != 120) {
		json << "\"distance\":{"
		     << "\"default\":" << (int)rip_cfg->distance
		     << "},";
	}

	/* Timers */
	if (rip_cfg->timers.update != 30 || rip_cfg->timers.invalid != 180 ||
	    rip_cfg->timers.flush != 240) {
		json << "\"timers\":{"
		     << "\"update\":" << rip_cfg->timers.update << ","
		     << "\"timeout\":" << rip_cfg->timers.invalid << ","
		     << "\"garbage-collection\":" << rip_cfg->timers.flush
		     << "},";
	}

	/* Default route origination */
	if (rip_cfg->default_route) {
		json << "\"default-information-originate\":true,";
	}

	/* Note: Debug settings are handled via system commands in netd,
	 * not via northbound API, since FRR doesn't support debug config */

	/* Interfaces (network statements) */
	struct rip_network *net;
	first = true;
	TAILQ_FOREACH(net, &rip_cfg->networks, entries) {
		if (first) {
			json << "\"interface\":[";
			first = false;
		} else {
			json << ",";
		}
		json << "\"" << net->ifname << "\"";
	}
	if (!first)
		json << "],";

	/* Explicit neighbors */
	struct rip_neighbor *nbr;
	first = true;
	TAILQ_FOREACH(nbr, &rip_cfg->neighbors, entries) {
		if (first) {
			json << "\"explicit-neighbor\":[";
			first = false;
		} else {
			json << ",";
		}
		inet_ntop(AF_INET, &nbr->addr, buf, sizeof(buf));
		json << "\"" << buf << "\"";
	}
	if (!first)
		json << "],";

	/* Redistribution */
	struct rip_redistribute *redist;
	first = true;
	TAILQ_FOREACH(redist, &rip_cfg->redistributes, entries) {
		if (first) {
			json << "\"redistribute\":[";
			first = false;
		} else {
			json << ",";
		}

		const char *proto = NULL;
		switch (redist->type) {
		case RIP_REDIST_CONNECTED:
			proto = "connected";
			break;
		case RIP_REDIST_STATIC:
			proto = "static";
			break;
		case RIP_REDIST_KERNEL:
			proto = "kernel";
			break;
		case RIP_REDIST_OSPF:
			proto = "ospf";
			break;
		}

		if (proto) {
			json << "{"
			     << "\"protocol\":\"" << proto << "\""
			     << "}";
		}
	}
	if (!first)
		json << "],";
	/* Passive interfaces - use /frr-interface augmentation */
	first = true;
	TAILQ_FOREACH(net, &rip_cfg->networks, entries) {
		if (!net->passive)
			continue;
		if (first) {
			json << "\"passive-interface\":[";
			first = false;
		} else {
			json << ",";
		}
		json << "\"" << net->ifname << "\"";
	}
	if (!first)
		json << "],";

	/* Remove trailing comma if present */
	std::string result = json.str();
	if (!result.empty() && result.back() == ',')
		result.pop_back();

	return result + "}]}}";

}

/*
 * Build complete routing configuration JSON with both static routes and RIP
 * Static routes go in /frr-routing:routing/control-plane-protocols
 * RIP goes in /frr-ripd:ripd/instance (top level, separate container)
 */
static std::string build_routing_json(struct route_head *routes, struct rip_config *rip_cfg)
{
	std::ostringstream json;
	bool has_static = routes && !TAILQ_EMPTY(routes);
	bool has_rip = rip_cfg && rip_cfg->enabled;

	json << "{";

	/* Add static routes */
	if (has_static) {
		json << "\"frr-routing:routing\":{"
		     << "\"control-plane-protocols\":{"
		     << "\"control-plane-protocol\":[";
		/* Build staticd part (extract from build_staticd_json) */
		struct route *r, *next;
		char buf[INET6_ADDRSTRLEN];
		bool first_route = true;

		json << "{"
		     << "\"type\":\"frr-staticd:staticd\","
		     << "\"name\":\"staticd\","
		     << "\"vrf\":\"default\","
		     << "\"frr-staticd:staticd\":{"
		     << "\"route-list\":[";

		r = TAILQ_FIRST(routes);
		while (r != NULL) {
			const char *afi;

			if (r->family == AF_INET) {
				inet_ntop(AF_INET, &r->prefix.ip4, buf, sizeof(buf));
				afi = "frr-routing:ipv4-unicast";
			} else {
				inet_ntop(AF_INET6, &r->prefix.ip6, buf, sizeof(buf));
				afi = "frr-routing:ipv6-unicast";
			}

			if (!first_route)
				json << ",";
			first_route = false;

			json << "{"
			     << "\"prefix\":\"" << buf << "/" << (int)r->prefixlen << "\","
			     << "\"src-prefix\":\"::/0\","
			     << "\"afi-safi\":\"" << afi << "\","
			     << "\"path-list\":[";

			bool first_path = true;
			struct route *curr = r;
			while (curr != NULL && same_prefix(r, curr)) {
				if (!first_path)
					json << ",";
				first_path = false;

				json << "{"
				     << "\"table-id\":0,"
				     << "\"distance\":" << (int)curr->distance << ","
				     << "\"frr-nexthops\":{\"nexthop\":[{";

				switch (curr->nh_type) {
				case NH_ADDR:
					if (curr->family == AF_INET)
						inet_ntop(AF_INET, &curr->gateway.gw4, buf, sizeof(buf));
					else
						inet_ntop(AF_INET6, &curr->gateway.gw6, buf, sizeof(buf));

					json << "\"nh-type\":\"" << (curr->family == AF_INET ? "ip4" : "ip6") << "\","
					     << "\"vrf\":\"default\","
					     << "\"gateway\":\"" << buf << "\","
					     << "\"interface\":\"\"";
					break;

				case NH_IFNAME:
					json << "\"nh-type\":\"ifindex\","
					     << "\"vrf\":\"default\","
					     << "\"gateway\":\"\","
					     << "\"interface\":\"" << curr->ifname << "\"";
					break;

				case NH_BLACKHOLE:
					json << "\"nh-type\":\"blackhole\","
					     << "\"vrf\":\"default\","
					     << "\"gateway\":\"\","
					     << "\"interface\":\"\",";

					switch (curr->bh_type) {
					case BH_DROP:
						json << "\"bh-type\":\"null\"";
						break;
					case BH_REJECT:
						json << "\"bh-type\":\"reject\"";
						break;
					case BH_NULL:
						json << "\"bh-type\":\"unspec\"";
						break;
					}
					break;
				}

				json << "}]}}";
				curr = TAILQ_NEXT(curr, entries);
			}

			json << "]}";
			r = curr;
		}

		json << "]}}]}}";
	}

	/* Add RIP config (separate top-level container) */
	if (has_rip) {
		if (has_static)
			json << ",";

		std::string rip_json = build_rip_json(rip_cfg);
		/* Remove the outer braces from rip_json since we're merging */
		if (rip_json.length() > 2) {
			json << rip_json.substr(1, rip_json.length() - 2);
		}
	}

	json << "}";

	return json.str();
}

extern "C" int grpc_backend_init(void)
{
	grpc::ChannelArguments args;
	
	/* Create insecure channel to local mgmtd */
	g_channel = grpc::CreateChannel(GRPC_SERVER, grpc::InsecureChannelCredentials());
	if (!g_channel) {
		ERROR("grpc: failed to create channel");
		return -1;
	}

	/* Create Northbound stub */
	g_stub = frr::Northbound::NewStub(g_channel);
	if (!g_stub) {
		ERROR("grpc: failed to create stub");
		g_channel.reset();
		return -1;
	}

	/* Test connection with GetCapabilities */
	frr::GetCapabilitiesRequest cap_req;
	frr::GetCapabilitiesResponse cap_resp;
	grpc::ClientContext cap_ctx;

	grpc::Status status = g_stub->GetCapabilities(&cap_ctx, cap_req, &cap_resp);
	if (!status.ok()) {
		ERROR("grpc: GetCapabilities failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());
		g_stub.reset();
		g_channel.reset();
		return -1;
	}

	INFO("grpc: connected to FRR mgmtd (version=%s, supported encodings=%d)",
	     cap_resp.frr_version().c_str(), cap_resp.supported_encodings_size());

	return 0;
}

extern "C" void grpc_backend_fini(void)
{
	/* Clean up any pending candidates */
	if (g_candidate_id != 0 && g_stub) {
		frr::DeleteCandidateRequest del_req;
		frr::DeleteCandidateResponse del_resp;
		grpc::ClientContext del_ctx;

		del_req.set_candidate_id(g_candidate_id);
		g_stub->DeleteCandidate(&del_ctx, del_req, &del_resp);
		g_candidate_id = 0;
	}

	g_stub.reset();
	g_channel.reset();

	DEBUG("grpc: finalized");
}

extern "C" int grpc_backend_apply(struct route_head *routes, struct rip_config *rip)
{
	if (!g_stub || !g_channel) {
		ERROR("grpc: not initialized");
		return -1;
	}

	/* Build JSON configuration for both static routes and RIP */
	std::string json_config = build_routing_json(routes, rip);
	if (json_config.empty()) {
		ERROR("grpc: failed to build JSON config");
		return -1;
	}

	DEBUG("grpc: JSON config: %s", json_config.c_str());

	/* Step 1: CreateCandidate */
	frr::CreateCandidateRequest create_req;
	frr::CreateCandidateResponse create_resp;
	grpc::ClientContext create_ctx;

	grpc::Status status = g_stub->CreateCandidate(&create_ctx, create_req, &create_resp);
	if (!status.ok()) {
		ERROR("grpc: CreateCandidate failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());
		return -1;
	}

	g_candidate_id = create_resp.candidate_id();
	DEBUG("grpc: created candidate %lu", (unsigned long)g_candidate_id);

	/* Step 2: LoadToCandidate (REPLACE entire routing config) */
	frr::LoadToCandidateRequest load_req;
	frr::LoadToCandidateResponse load_resp;
	grpc::ClientContext load_ctx;

	load_req.set_candidate_id(g_candidate_id);
	load_req.set_type(frr::LoadToCandidateRequest_LoadType_REPLACE);

	auto* config = load_req.mutable_config();
	config->set_encoding(frr::JSON);
	config->set_data(json_config);

	status = g_stub->LoadToCandidate(&load_ctx, load_req, &load_resp);
	if (!status.ok()) {
		ERROR("grpc: LoadToCandidate failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());

		/* Clean up candidate */
		frr::DeleteCandidateRequest del_req;
		frr::DeleteCandidateResponse del_resp;
		grpc::ClientContext del_ctx;
		del_req.set_candidate_id(g_candidate_id);
		g_stub->DeleteCandidate(&del_ctx, del_req, &del_resp);
		g_candidate_id = 0;

		return -1;
	}

	DEBUG("grpc: config loaded to candidate");

	/* Step 3: Commit */
	frr::CommitRequest commit_req;
	frr::CommitResponse commit_resp;
	grpc::ClientContext commit_ctx;

	commit_req.set_candidate_id(g_candidate_id);
	commit_req.set_phase(frr::CommitRequest_Phase_ALL);
	commit_req.set_comment("netd configuration (routes + rip)");

	status = g_stub->Commit(&commit_ctx, commit_req, &commit_resp);
	if (!status.ok()) {
		/* ABORTED with "No changes to apply" is a success case */
		if (status.error_code() == grpc::StatusCode::ABORTED &&
		    status.error_message().find("No changes to apply") != std::string::npos) {
			DEBUG("grpc: No changes to apply (config already up-to-date)");

			/* Clean up candidate - it won't be auto-deleted on ABORTED */
			frr::DeleteCandidateRequest del_req;
			frr::DeleteCandidateResponse del_resp;
			grpc::ClientContext del_ctx;
			del_req.set_candidate_id(g_candidate_id);
			g_stub->DeleteCandidate(&del_ctx, del_req, &del_resp);
			g_candidate_id = 0;

			INFO("grpc: configuration already up-to-date");
			return 0;
		}

		ERROR("grpc: Commit failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());

		/* Clean up candidate */
		frr::DeleteCandidateRequest del_req;
		frr::DeleteCandidateResponse del_resp;
		grpc::ClientContext del_ctx;
		del_req.set_candidate_id(g_candidate_id);
		g_stub->DeleteCandidate(&del_ctx, del_req, &del_resp);
		g_candidate_id = 0;

		return -1;
	}

	/* Candidate is auto-deleted after successful commit */
	g_candidate_id = 0;

	INFO("grpc: configuration committed successfully");
	return 0;
}
