#include <linux/path.h>

typedef struct drepl drepl;
typedef struct drepl_block drepl_block;
typedef struct drepl_dest drepl_dest;
typedef struct drepl_expr drepl_expr;
typedef struct drepl_repl drepl_repl;
typedef struct drepl_view drepl_view;
typedef struct drepl_write_req drepl_write_req;

#define ROWMAJOR	1
#define ROWMINOR	2

// view flags
#define VSYNC		1

struct drepl {
        int		nrepls;
        drepl_repl*	repls;
        int		nviews;
        drepl_view*	views;
        int		nblks;
        drepl_block*	blks;

        // runtime data
        // TODO
};

struct drepl_repl {
        int		id;
        char*		name;
        char*		fname;

        // runtime data
        struct file*	file;
        struct path    f_path;
        const struct vm_operations_struct* vm_ops;
};

struct drepl_view {
        int		id;
        char*		name;
        int		flags;
        drepl_repl*	repl;
        u64		offset;
        u32		elo;
        drepl_view*	dflt;
        u64		size;
        u32		nblks;
        drepl_block**	blks;

        // runtime data
        // TODO
};

struct drepl_expr {
        s64		a;
        s64		b;
        s64		c;
        s64		d;
        u32		xidx;
};

struct drepl_dest {
        int		nexpr;
        drepl_expr*	expr;
        drepl_block*	arr;
        drepl_block*	el;
};

struct drepl_block {
        u32		id;
        drepl_view*	view;
        u64		offset;
        u64		size;
        drepl_dest	src;
        u32		ndest;
        drepl_dest*	dest;

        // ablock
        u32		ndim;
        u64*		dim;
        u64		elsize;
        u64		elnum;
        drepl_block*	el;

        // tblock
        u32		nfld;
        drepl_block**	fld;
};

struct drepl_write_req {
        drepl_view*		v;
        int			bidx;
        u64			offset;
        u64			datalen;
        const char __user*	data;
        struct work_struct	work;
        wait_queue_head_t*	wq;
        atomic_t*		rcnt;
        mm_segment_t		fs;
};

extern struct workqueue_struct *drepl_workqueue;

/* import.c */
extern drepl *drepl_import(u8 *buf);

/* block.c */
extern s64 drepl_block_read(drepl_block *b, u8 __user *data, u64 datalen, u64 offset, u64 base);
extern s64 drepl_block_write(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base, int w, int r);
extern s64 drepl_block_replicate(drepl_block *b, const char __user *data, u64 datalen, u64 offset, u64 base);

/* elo.c */
extern void drepl_elo_toidx(int elo, s64 n, int ndim, s64 *idx, s64 *dim);
extern s64 drepl_elo_fromidx(int elo, int ndim, s64 *idx, s64 *dim);

/* expr.c */
void drepl_calc_expr(drepl_expr *p, s64 *xa, s64 *q, s64 *r);

/* repl.c */
extern s64 drepl_repl_write(drepl_repl *r, const char __user *data, u64 datalen, u64 offset);
extern s64 drepl_repl_read(drepl_repl *r, char __user *data, u64 datalen, u64 offset);
extern int drepl_repl_init(drepl_repl *r);
extern void drepl_repl_destroy(drepl_repl *r);

/* view.c */
extern int drepl_search_start(drepl_view *v, u64 offset);
extern s64 drepl_view_write(drepl_view *v, const char __user *data, u64 datalen, u64 offset);
extern s64 drepl_view_read(drepl_view *v, char __user *data, u64 datalen, u64 offset);
