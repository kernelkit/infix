#include <errno.h>
#include <grp.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <libyang/libyang.h>

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

int unpercent(char *text)
{
	char *in, *out, *end;
	int escaped;

	in = out = text;
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

struct uri_node {
	char *module;
	char *name;
	char *keyv[8];
};

int uri_node_from_seg(char *seg, struct uri_node *n)
{
	int keyc = 0;
	char *delim;

	memset(n, 0, sizeof(*n));

	delim = strchr(seg, ':');
	if (delim) {
		*delim++ = '\0';

		n->module = seg;
		n->name = seg = delim;
	} else {
		n->name = seg;
	}

	delim = strchr(seg, '=');
	if (!delim)
		return 0;

	do {
		if (keyc >= 7)
			return -EINVAL;

		*delim++ = '\0';
		n->keyv[keyc++] = seg = delim;
	} while ((delim = strchr(seg, ','); keyc++));

	return 0;
}

int xpath_from_uri(struct ly_ctx *ly, char *uri, char **xpathp)
{
	
}


int main(void)
{
	
}
