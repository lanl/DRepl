/*
 * Copyright (c) 1998-2011 Erez Zadok
 * Copyright (c) 2009	   Shrikar Archak
 * Copyright (c) 2003-2011 Stony Brook University
 * Copyright (c) 2003-2011 The Research Foundation of SUNY
 * Copyright (c) 2012 Latchesar Ionkov <lionkov@lanl.gov>
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 2 as
 * published by the Free Software Foundation.
 */

#include <linux/fs.h>
#include "drepl.h"
#include "dreplfs.h"

/*
 * returns: -ERRNO if error (returned to user)
 *          0: tell VFS to invalidate dentry
 *          1: dentry is valid
 */
static int dreplfs_d_revalidate(struct dentry *dentry, unsigned int flags)
{
	printk(KERN_DEFAULT "dreplfs_d_revalidate\n");
	if (flags & LOOKUP_RCU)
		return -ECHILD;

	return 1;
}

const struct dentry_operations dreplfs_dops = {
	.d_revalidate	= dreplfs_d_revalidate,
};
