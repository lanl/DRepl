#!/bin/bash

SIZES="1 2 4 8 16 64 128 256 511"
FLUSHDEV=/dev/sda
CHUNKSZ=1024

for sz in `echo $SIZES` ; do
	SZ=`expr $sz '*' 1048576`
	for op in w R W r ; do
		# flush buffer cache
		dd if=$FLUSHDEV of=/dev/null bs=1048576 count=65536 2>/dev/null

		# run the test
		/home/lionkov/drepl/atest -s $SZ -c $CHUNKSZ -f /dev/sdb $op 2>&1 >> /home/lionkov/drepl/results-sas
	done
done
