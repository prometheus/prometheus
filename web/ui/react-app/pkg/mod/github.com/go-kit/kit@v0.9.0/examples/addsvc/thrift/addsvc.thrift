struct SumReply {
	1: i64 value
	2: string err
}

struct ConcatReply {
	1: string value
	2: string err
}

service AddService {
	SumReply Sum(1: i64 a, 2: i64 b)
	ConcatReply Concat(1: string a, 2: string b)
}
