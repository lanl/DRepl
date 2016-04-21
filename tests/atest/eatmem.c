#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void
usage()
{
	printf("Usage: eatmem <size-in-MB>\n");
	exit(1);
}

int
main(int argc, char **argv)
{
	unsigned long long sz, n;
	char *s, *b;

	if (argc != 2)
		usage();

	sz = strtol(argv[1], &s, 10);
	if (*s != '\0')
		usage();

	sz *= 1024*1024;

	while (sz > 0) {
		n = 1024*1024*1024;
		if (n > sz) {
			n = sz;
		}

		b = malloc(n);
		memset(b, 42, n);
		sz -= n;
	}

	while (1) {
		sleep(1000);
	}

	return 0;
}
