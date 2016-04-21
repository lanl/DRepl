#include <linux/slab.h>
#include "drepl.h"

void drepl_elo_toidx(int elo, s64 n, int ndim, s64 *idx, s64 *dim)
{
	int i;

	switch (elo) {
	case ROWMAJOR:
		for(i = ndim - 1; i >= 0; i--) {
			idx[i] = n % dim[i];
			n /= dim[i];
		}
		break;

	case ROWMINOR:
		for(i = 0; i < ndim; i++) {
			idx[i] = n % dim[i];
			n /= dim[i];
		}
		break;
	}
}

s64 drepl_elo_fromidx(int elo, int ndim, s64 *idx, s64 *dim)
{
	int i;
	s64 n;

	n = 0;
	switch (elo) {
	case ROWMAJOR:
		n = idx[0];
		for(i = 1; i < ndim; i++) {
			n = n*dim[i] + idx[i];
		}
		break;

	case ROWMINOR:
		n = idx[ndim - 1];
		for(i = ndim - 2; i >= 0; i--) {
			n = n*dim[i] + idx[i];
		}
		break;
	}

	return n;
}
