/*
 * LF: Global Fully Replicated Key/Value Store
 * Copyright (C) 2018  ZeroTier, Inc.  https://www.zerotier.com/
 * 
 * Licensed under the terms of the MIT license (see LICENSE.txt).
 */

#ifndef ZTLF_ISET_H
#define ZTLF_ISET_H

#include "common.h"
#include "vector.h"

#define ZTLF_ISET_BUCKET_COUNT 8388608

struct ZTLF_ISet { struct ZTLF_Vector_i64 buckets[ZTLF_ISET_BUCKET_COUNT]; };

static inline struct ZTLF_ISet *ZTLF_ISet_new()
{
	struct ZTLF_ISet *s;
	ZTLF_MALLOC_CHECK(s = (struct ZTLF_ISet *)malloc(sizeof(struct ZTLF_ISet)));
	memset(s,0,sizeof(struct ZTLF_ISet));
	return s;
}

static inline void ZTLF_ISet_free(struct ZTLF_ISet *s)
{
	for(unsigned long k=0;k<ZTLF_ISET_BUCKET_COUNT;++k) {
		ZTLF_Vector_i64_free(s->buckets + k);
	}
	free(s);
}

static inline void ZTLF_ISet_clear(struct ZTLF_ISet *s)
{
	for(unsigned long k=0;k<ZTLF_ISET_BUCKET_COUNT;++k) {
		ZTLF_Vector_i64_clear(s->buckets + k);
	}
}

static inline bool ZTLF_ISet_put(struct ZTLF_ISet *s,const int64_t i)
{
	struct ZTLF_Vector_i64 *const v = s->buckets + ((unsigned long)ZTLF_xorshift64starOnce((uint64_t)i) % ZTLF_ISET_BUCKET_COUNT);
	for(unsigned long k=0;k<v->size;k++) {
		if (v->v[k] == i)
			return false;
	}
	ZTLF_Vector_i64_append(v,i);
	return true;
}

static inline bool ZTLF_ISet_contains(struct ZTLF_ISet *s,const int64_t i)
{
	struct ZTLF_Vector_i64 *const v = s->buckets + ((unsigned long)ZTLF_xorshift64starOnce((uint64_t)i) % ZTLF_ISET_BUCKET_COUNT);
	for(unsigned long k=0;k<v->size;k++) {
		if (v->v[k] == i)
			return true;
	}
	return false;
}

#endif