#!/bin/bash

for i in 1 2 3 ; do
	/home/lionkov/drepl/run-sas.sh $i
	/home/lionkov/drepl/run-ssd.sh $i
done

