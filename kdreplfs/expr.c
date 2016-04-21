#include <linux/slab.h>
#include "drepl.h"

void drepl_calc_expr(drepl_expr *p, s64 *xa, s64 *q, s64 *r)
{
	s64 x, n, m;

	x = xa[p->xidx];

	// TODO: handle overflows
	n = x*p->a + p->b;
	m = x*p->c + p->d;
	*q = n / m;
	*r = n % m;
}

