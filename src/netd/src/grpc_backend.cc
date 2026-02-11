/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * gRPC backend for netd - communicates with FRR's management daemon
 * using the gRPC northbound API. This provides a standard protocol
 * interface for configuration management.
 */

#include <memory>

#include <grpcpp/grpcpp.h>
#include <grpcpp/create_channel.h>
#include <grpcpp/security/credentials.h>

#include "grpc/frr-northbound.grpc.pb.h"
#include "grpc/frr-northbound.pb.h"

extern "C" {
#include "netd.h"
#include "grpc_backend.h"
#include "json_builder.h"
}

/* Global gRPC state */
static std::shared_ptr<grpc::Channel> g_channel;
static std::unique_ptr<frr::Northbound::Stub> g_stub;
static uint64_t g_candidate_id = 0;

/* gRPC server address */
#define GRPC_SERVER "127.0.0.1:50051"

extern "C" int grpc_backend_init(void)
{
	frr::GetCapabilitiesResponse cap_resp;
	frr::GetCapabilitiesRequest cap_req;
	grpc::ClientContext cap_ctx;
	grpc::ChannelArguments args;
	grpc::Status status;

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
	status = g_stub->GetCapabilities(&cap_ctx, cap_req, &cap_resp);
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

extern "C" void grpc_backend_cleanup(void)
{
	frr::DeleteCandidateResponse del_resp;
	frr::DeleteCandidateRequest del_req;
	grpc::ClientContext del_ctx;

	/* Clean up any pending candidates */
	if (g_candidate_id != 0 && g_stub) {
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
	frr::CreateCandidateResponse create_resp;
	frr::LoadToCandidateResponse load_resp;
	frr::CreateCandidateRequest create_req;
	frr::DeleteCandidateResponse del_resp;
	frr::LoadToCandidateRequest load_req;
	frr::DeleteCandidateRequest del_req;
	frr::CommitResponse commit_resp;
	grpc::ClientContext create_ctx;
	grpc::ClientContext commit_ctx;
	frr::CommitRequest commit_req;
	grpc::ClientContext load_ctx;
	grpc::ClientContext del_ctx;
	const char *json_config;
	grpc::Status status;

	if (!g_stub || !g_channel) {
		ERROR("grpc: not initialized");
		return -1;
	}

	/* Build JSON configuration for both static routes and RIP */
	json_config = build_routing_json(routes, rip);
	if (!json_config) {
		ERROR("grpc: failed to build JSON config");
		return -1;
	}

	DEBUG("grpc: JSON config: %s", json_config);

	/* Step 1: CreateCandidate */
	status = g_stub->CreateCandidate(&create_ctx, create_req, &create_resp);
	if (!status.ok()) {
		ERROR("grpc: CreateCandidate failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());
		return -1;
	}

	g_candidate_id = create_resp.candidate_id();
	DEBUG("grpc: created candidate %lu", (unsigned long)g_candidate_id);

	/* Step 2: LoadToCandidate (REPLACE entire routing config) */
	load_req.set_candidate_id(g_candidate_id);
	load_req.set_type(frr::LoadToCandidateRequest_LoadType_REPLACE);

	{
		auto* config = load_req.mutable_config();
		config->set_encoding(frr::JSON);
		config->set_data(json_config);
	}

	status = g_stub->LoadToCandidate(&load_ctx, load_req, &load_resp);
	if (!status.ok()) {
		ERROR("grpc: LoadToCandidate failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());

		/* Clean up candidate */
		del_req.set_candidate_id(g_candidate_id);
		g_stub->DeleteCandidate(&del_ctx, del_req, &del_resp);
		g_candidate_id = 0;

		return -1;
	}

	DEBUG("grpc: config loaded to candidate");

	/* Step 3: Commit */
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
			del_req.set_candidate_id(g_candidate_id);
			g_stub->DeleteCandidate(&del_ctx, del_req, &del_resp);
			g_candidate_id = 0;

			INFO("grpc: configuration already up-to-date");
			return 0;
		}

		ERROR("grpc: Commit failed: %s (code=%d)",
		      status.error_message().c_str(), status.error_code());

		/* Clean up candidate */
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
