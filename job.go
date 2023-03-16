// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "sync"

var jobEmpty job

type job struct {
	msg  *Message
	next *job
}

var jobPool = sync.Pool{
	New: func() interface{} {
		return &job{}
	},
}

func getJob() *job {
	return jobPool.Get().(*job)
}

func putJob(jo *job) {
	*jo = jobEmpty
	jobPool.Put(jo)
}
