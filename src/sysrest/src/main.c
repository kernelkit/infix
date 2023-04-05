#include <errno.h>
#include <grp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <sys/stat.h>

#include <fcgiapp.h>

#include <libyang/libyang.h>
#include <libyang/plugins_exts.h>

#include <sysrepo.h>
#include <sysrepo_types.h>

static const char *rootpath = "/restconf/";

enum http_method {
	HTTP_GET,
	HTTP_HEAD,
	HTTP_PATCH,
	HTTP_POST,
	HTTP_PUT,

	HTTP_INVALID
};

/* struct memstream { */
/* 	FILE *fp; */
/* 	char *buf; */
/* 	size_t len; */
/* }; */

/* struct res { */
/* 	struct memstream head; */
/* 	struct memstream entity; */
/* }; */

/* int res_push_header(struct res *res, const char *key, const char *valfmt, ...) */
/* { */
/* 	fprintf */
/* } */

/* int res_respond_to(struct res *res, FCGX_Request *req) */
/* { */
	
/* } */

/* int res_init(struct res *res) */
/* { */
/* 	res->head.fp = open_memstream(&res->head.buf, &res->head.len); */
/* 	if (!res->head.fp) */
/* 		goto err; */

/* 	res->entity.fp = open_memstream(&res->entity.buf, &res->entity.len); */
/* 	if (!res->entity.fp) */
/* 		goto err_free_head; */

/* 	return 0; */

/* err_free_head: */
/* 	fclose(res->head.fp); */
/* err: */
/* 	return -ENOMEM; */
/* } */

struct req {
	FCGX_Request r;

	enum http_method method;
	char *uri;

	union {
		struct {
			char *path;
		} data;
	};
};

static enum http_method req_parse_method(struct req *req)
{
	const char *str = FCGX_GetParam("REQUEST_METHOD", req->r.envp);

	if (!str)
		return HTTP_INVALID;

	if (!strcmp(str, "GET"))
		return HTTP_GET;
	if (!strcmp(str, "HEAD"))
		return HTTP_HEAD;
	if (!strcmp(str, "PATCH"))
		return HTTP_PATCH;
	if (!strcmp(str, "POST"))
		return HTTP_POST;
	if (!strcmp(str, "PUT"))
		return HTTP_PUT;

	return HTTP_INVALID;
}

static int req_init(struct req *req)
{
	const char *uri;

	req->method = req_parse_method(req);

	uri = FCGX_GetParam("REQUEST_URI", req->r.envp);
	if (!uri)
		return -EINVAL;

	req->uri = strdup(uri);
	if (!req->uri)
		return -EINVAL;

	return 0;
}

static int req_fini(struct req *req)
{
	free(req->uri);
	return 0;
}

static void dumpenv(char **envp)
{
	fprintf(stderr, "ENV:\n");
	for (; *envp; envp++) {
		fprintf(stderr, "  %s\n", *envp);
	}
}

int serve_host_meta(struct req *req)
{
	size_t len;
	char *buf;
	FILE *fp;

	switch (req->method) {
	case HTTP_GET:
	case HTTP_HEAD:
		break;
	default:
		return -EINVAL;
	}

	fp = open_memstream(&buf, &len);
	if (!fp)
		return -ENOMEM;

	fprintf(fp,
		"<XRD xmlns='http://docs.oasis-open.org/ns/xri/xrd-1.0'>"
		"<Link rel='restconf' href='%s'/>"
		"</XRD>", rootpath);
	fclose(fp);

	FCGX_FPrintF(req->r.out, "Content-Type: application/xrd+xml\r\n");
	FCGX_FPrintF(req->r.out, "Content-Length: %zd\r\n", len);
	FCGX_FPrintF(req->r.out, "Connection: close\r\n");
	FCGX_PutS("\r\n", req->r.out);

	if (req->method == HTTP_GET)
		FCGX_PutStr(buf, len, req->r.out);

	free(buf);

	FCGX_Finish_r(&req->r);
	return 0;
}

int hex2bin(char *hex)
{
	int i, out;

	for (i = 0, out = 0; i < 2; i++) {
		out <<= 4;

		if (hex[i] >= '0' && hex[i] <= '9')
			out |= hex[0] - '0';
		else if (hex[i] >= 'a' && hex[i] <= 'f')
			out |= hex[0] - 'a';
		else if (hex[i] >= 'A' && hex[i] <= 'F')
			out |= hex[0] - 'A';
		else
			return -EINVAL;
	}

	if (out < 0 || out >= 0x80)
		/* No UTF-8 support */
		return -EINVAL;

	return out;
}

int unpercent(char *tok)
{
	char *in, *out, *end;
	int escaped;

	in = out = tok;
	for (; *in; in++, out++) {
		if (*in != '%') {
			*out = *in;
			continue;
		}

		escaped = hex2bin(in + 1);
		if (escaped < 0)
			return escaped;

		*out = escaped;
		in += 2;
	}

	*out = '\0';
	return 0;
}

int xpath_from_uri(struct ly_ctx *ly, char *uri, char **xpathp)
{
	
}

int uri_resolve_r(sr_data_t **nodep, char **urip, char **toksave)
{
	char *tok;
	int err;

	tok = strtok_r(*toksave ? NULL : *urip, "/", toksave);
	if (!tok)
		return 0;

	err = unpercent(tok);
	if (err)
		return err;

	
}

int uri_resolve(sr_session_ctx_t *sess, char *uri, struct lyd_node **nodep)
{
	sr_data_t *node;
	char *toksave = NULL;
	int err;

	err = sr_get_subtree(sess, "/", 0, &node);
	if (err)
		return err;

	err = uri_resolve_r(&node, &uri, &toksave);
	if (err)
		return err;

	tok = srst_uritok(uri);
	if (!tok)
		return 0;

}

int serve_restconf_data(struct req *req)
{
	sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn;
	sr_data_t *data;
	int err = 0;

	FILE *fp;
	char *buf;
	size_t len = 0;

	if (req->method != HTTP_GET) {
		err = -EINVAL;
		goto err;
	}

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		err = -EIO;
		goto err;
	}

	if (sr_session_start(conn, SR_DS_RUNNING, &sess)) {
		err = -EIO;
		goto err_disconnect;
	}


	sr_get_data(sess, req->data.path, 0, 0, 0, &data);

	fp = open_memstream(&buf, &len);
	lyd_print_file(fp, data ? data->tree : NULL, LYD_JSON, LYD_PRINT_WITHSIBLINGS);
	fclose(fp);

	FCGX_FPrintF(req->r.out, "Content-Type: application/yang-data+json\r\n");
	FCGX_FPrintF(req->r.out, "Content-Length: %zd\r\n", len);
	FCGX_FPrintF(req->r.out, "Connection: close\r\n");
	FCGX_PutS("\r\n", req->r.out);

	FCGX_PutStr(buf, len, req->r.out);
	free(buf);

	FCGX_Finish_r(&req->r);
	return 0;

err_disconnect:
	sr_disconnect(conn);
err:
	return err;
}

int serve_restconf(struct req *req)
{
	char *uri = req->uri + strlen(rootpath);

	if (!strncmp(uri, "data/", strlen("data/"))) {
		req->data.path = uri + strlen("data/");
		return serve_restconf_data(req);
	}

	return -EINVAL;
}

int serve(int sd)
{
	struct req req;
	int err = 0;

	for (FCGX_InitRequest(&req.r, sd, 0);
	     FCGX_Accept_r(&req.r) == 0;
	     FCGX_InitRequest(&req.r, sd, 0)) {
		if (req_init(&req))
			continue;

		dumpenv(req.r.envp);

		if (!strcmp(req.uri, "/.well-known/host-meta"))
			err = serve_host_meta(&req);
		else if (!strncmp(req.uri, rootpath, strlen(rootpath)))
			err = serve_restconf(&req);
		else
			err = -EINVAL;

		if (err) {
			FCGX_FPrintF(req.r.out, "Status: 400 Bad Request\r\n");
			FCGX_FPrintF(req.r.out, "Content-Type: text/html\r\n");
			FCGX_FPrintF(req.r.out, "Connection: close\r\n");
			FCGX_PutS("\r\n", req.r.out);
			FCGX_Finish_r(&req.r);
		}

		req_fini(&req);
	}

	FCGX_Free(&req.r, sd);

	if (err)
		fprintf(stderr, "Unable to handle request: %d\n", err);

	return err ? 1 : 0;
}

int setup_sock(const char *sockpath, const char *groupname)
{
	struct group *g;
	int sd;

	sd = FCGX_OpenSocket(sockpath, 10);
	if (sd < 0) {
		perror("Unable to create socket");
		return -1;
	}

	g = getgrnam(groupname);
	if (!g)
		return -1;

	if (chown(sockpath, -1, g->gr_gid)) {
		perror("Unable to set socket group");
		return -1;
	}

	if (chmod(sockpath, S_IRWXU|S_IRWXG|S_IROTH) < 0){
		perror("Unable to set socket permissions");
		return -1;
	}

	return sd;
}

int main(int argc, char **argv, char **envp)
{
	int sd;

	if (FCGX_Init()) {
		perror("Unable to initialize libfcgi");
		return 1;
	}

	sd = setup_sock("/run/sysrest.sock", "www-data");
	if (sd < 0)
		return 1;

	return serve(sd);
}
