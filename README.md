# DRepl

DRepl is a project that attempts to capture the storage and
communication requirements of a scientific application at a higher
level than most of the libraries. It allows the developers to define a
dataset, how various applications (and partitions of an application)
use subsets of the data., as well as how same or other subsets of the
data are saved on persistent storage. DRepl consists of a dataset
description language, parser that produces transformation rules, as
well as Linux kernel filesystem for accessing the defined subset of
the data. The language is used to define the logical dataset, the data
subsets (views) used by the applications, as well as the data layout
on persistent storage (replicas). The parser uses the language
description to produce transformation rules that are used by the
dreplfs kernel filesystem to present the dataset as materialized or
unmaterialized views. DRepl allows applications' dataset
requirements to be isolated from the physical storage systems,
allowing optimizations that will improve storage performance at
exascale.

# Release

This software has been approved for open source release and has been
assigned **LA-CC-16-45**.

# Copyright

Copyright (c) 2015, Los Alamos National Security, LLC
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

1. Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright
notice, this list of conditions and the following disclaimer in the
documentation and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

