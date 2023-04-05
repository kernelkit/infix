#include <errno.h>
#include <grp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <sys/stat.h>

#include <fcgiapp.h>

static const char *rootpath = "/restconf/";

static void dumpenv(char **envp)
{
	fprintf(stderr, "ENV:\n");
	for (; *envp; envp++) {
		fprintf(stderr, "  %s\n", *envp);
	}
}

int serve_host_meta(FCGX_Request *req)
{
	const char *method;
	size_t len;
	char *buf;
	FILE *fp;

	method = FCGX_GetParam("REQUEST_METHOD", req->envp);
	if (!method || (strcmp(method, "GET") && strcmp(method, "HEAD")))
		return -EINVAL;

	fp = open_memstream(&buf, &len);
	if (!fp)
		return -ENOMEM;

	fprintf(fp,
		"<XRD xmlns='http://docs.oasis-open.org/ns/xri/xrd-1.0'>"
		"<Link rel='restconf' href='%s'/>"
		"</XRD>", rootpath);
	fclose(fp);

	FCGX_FPrintF(req->out, "Content-Type: application/xrd+xml\r\n");
	FCGX_FPrintF(req->out, "Content-Length: %zd\r\n", len);
	FCGX_FPrintF(req->out, "Connection: close\r\n");
	FCGX_PutS("\r\n", req->out);

	if (!strcmp(method, "GET"))
		FCGX_PutStr(buf, len, req->out);

	free(buf);

	FCGX_Finish_r(req);
	return 0;
}

int serve_restconf(FCGX_Request *req)
{
	FCGX_FPrintF(req->out, "Content-type: text/html\r\n\r\n");
	FCGX_FPrintF(req->out, "RESTCONF\r\n");
	FCGX_Finish_r(req);
	return 0;
}

int serve(int sd)
{
	FCGX_Request req;
	const char *uri;
	int err = 0;

	for (FCGX_InitRequest(&req, sd, 0);
	     FCGX_Accept_r(&req) == 0;
	     FCGX_InitRequest(&req, sd, 0)) {
		dumpenv(req.envp);

		uri = FCGX_GetParam("REQUEST_URI", req.envp);
		if (!uri)
			continue;

		if (!strcmp(uri, "/.well-known/host-meta"))
			err = serve_host_meta(&req);
		else if (!strncmp(uri, rootpath, strlen(rootpath)))
			err = serve_restconf(&req);
		else
			err = -EINVAL;

		if (err) {
			FCGX_FPrintF(req.out, "Status: 400 Bad Request\r\n");
			FCGX_FPrintF(req.out, "Content-Type: text/html\r\n");
			FCGX_FPrintF(req.out, "Connection: close\r\n");
			FCGX_PutS("\r\n", req.out);
			FCGX_Finish_r(&req);
			break;
		}
	}

	FCGX_Free(&req, sd);

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
