#!/bin/bash

SIZES="1 2 4 8 16 64 128 256 511"
FLUSHDEV=/dev/sdb
CHUNKSZ=1024

cd /ssdsata
rm -f atest.data
dd if=/dev/zero of=atest.data bs=1048576 count=8192
for sz in `echo $SIZES` ; do
	SZ=`expr $sz '*' 1048576`
	for op in w R W r ; do
		# flush buffer cache
		dd if=$FLUSHDEV of=/dev/null bs=1048576 count=65536 2>/dev/null

		# run the test
		/home/lionkov/drepl/atest -s $SZ -c $CHUNKSZ $op >> /home/lionkov/drepl/results$1-ssd
	done
done
