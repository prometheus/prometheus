# AliyunGo: Go SDK for Aliyun Services

[![Build Status](https://travis-ci.org/denverdino/aliyungo.svg?branch=master)](https://travis-ci.org/denverdino/aliyungo) [![CircleCI](https://circleci.com/gh/denverdino/aliyungo.svg?style=svg)](https://circleci.com/gh/denverdino/aliyungo) [![Go Report Card](https://goreportcard.com/badge/github.com/denverdino/aliyungo)](https://goreportcard.com/report/github.com/denverdino/aliyungo)

This is an unofficial Go SDK for Aliyun services. You are welcome for contribution.

The official SDK for Aliyun services is published. Please visit https://github.com/aliyun/alibaba-cloud-sdk-go for details

## Package Structure

* cdn: [Content Delivery Network](https://help.aliyun.com/document_detail/27101.html)
* cms: [Cloud Monitor Service](https://help.aliyun.com/document_detail/28615.html)
* cs: [Container Service](https://help.aliyun.com/product/25972.html)
* dm: [Direct Mail](https://help.aliyun.com/document_detail/29414.html)
* dns: [DNS](https://help.aliyun.com/document_detail/dns/api-reference/summary.html)
* ecs: [Elastic Compute Service](https://help.aliyun.com/document_detail/ecs/open-api/summary.html)
* ess: [Auto Scaling](https://help.aliyun.com/document_detail/25857.html)
* mns: [Message Service](https://help.aliyun.com/document_detail/27414.html)
* mq: [Message Queue](https://help.aliyun.com/document_detail/29532.html)
* nas: [Network Attached Storage](https://help.aliyun.com/document_detail/27518.html)
* opensearch: [OpenSearch](https://help.aliyun.com/document_detail/29118.html)
* oss: [Open Storage Service](https://help.aliyun.com/document_detail/oss/api-reference/abstract.html)
* push: [Cloud Mobile Push](https://help.aliyun.com/document_detail/30049.html)
* rds: [Relational Database Service](https://help.aliyun.com/document_detail/26226.html)
* ram: [Resource Access Management](https://help.aliyun.com/document_detail/ram/ram-api-reference/intro/intro.html)
* slb: [Server Load Balancer](https://help.aliyun.com/document_detail/slb/api-reference/brief-introduction.html)
* sls: [Logging Service](https://help.aliyun.com/document_detail/sls/api/overview.html)
* sms: [Short Message Service](https://help.aliyun.com/product/44282.html)
* sts: [Security Token Service](https://help.aliyun.com/document_detail/28756.html)
* common: Common libary of Aliyun Go SDK
* util: Utility helpers

## Quick Start

```go
package main

import (
  "fmt"

	"github.com/denverdino/aliyungo/ecs"
)

const ACCESS_KEY_ID = "<YOUR_ID>"
const ACCESS_KEY_SECRET = "<****>"

func main() {
	client := ecs.NewClient(ACCESS_KEY_ID, ACCESS_KEY_SECRET)
	fmt.Print(client.DescribeRegions())
}

```

## Documentation

  * CDN: [https://godoc.org/github.com/denverdino/aliyungo/cdn](https://godoc.org/github.com/denverdino/aliyungo/cdn)[![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/cdn?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/cdn)
  * CMS: [https://godoc.org/github.com/denverdino/aliyungo/cms](https://godoc.org/github.com/denverdino/aliyungo/cms) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/cms?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/cms)
  * CS: [https://godoc.org/github.com/denverdino/aliyungo/cs](https://godoc.org/github.com/denverdino/aliyungo/cs) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/cs?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/cs)
  * DM: [https://godoc.org/github.com/denverdino/aliyungo/dm](https://godoc.org/github.com/denverdino/aliyungo/dm) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/dm?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/dm)
  * DNS: [https://godoc.org/github.com/denverdino/aliyungo/dns](https://godoc.org/github.com/denverdino/aliyungo/dns) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/dns?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/dns)
  * ECS: [https://godoc.org/github.com/denverdino/aliyungo/ecs](https://godoc.org/github.com/denverdino/aliyungo/ecs) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/ecs?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/ecs)
  * ESS: [https://godoc.org/github.com/denverdino/aliyungo/ess](https://godoc.org/github.com/denverdino/aliyungo/ess)[![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/ess?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/ess)
  * MNS: [https://godoc.org/github.com/denverdino/aliyungo/mns](https://godoc.org/github.com/denverdino/aliyungo/mns)[![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/mns?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/mns)
  * MQ: [https://godoc.org/github.com/denverdino/aliyungo/mq](https://godoc.org/github.com/denverdino/aliyungo/mq) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/mq?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/mq)
  * NAS: [https://godoc.org/github.com/denverdino/aliyungo/nas](https://godoc.org/github.com/denverdino/aliyungo/nas) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/nas?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/nas)
  * OPENSEARCH: [https://godoc.org/github.com/denverdino/aliyungo/opensearch](https://godoc.org/github.com/denverdino/aliyungo/opensearch) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/opensearch?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/opensearch)
  * OSS: [https://godoc.org/github.com/denverdino/aliyungo/oss](https://godoc.org/github.com/denverdino/aliyungo/oss) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/oss?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/oss)
  * PUSH: [https://godoc.org/github.com/denverdino/aliyungo/push](https://godoc.org/github.com/denverdino/aliyungo/push) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/push?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/push)
  * RAM: [https://godoc.org/github.com/denverdino/aliyungo/ram](https://godoc.org/github.com/denverdino/aliyungo/ram) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/ram?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/ram)
  * RDS: [https://godoc.org/github.com/denverdino/aliyungo/rds](https://godoc.org/github.com/denverdino/aliyungo/rds) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/rds?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/rds)
  * SLB: [https://godoc.org/github.com/denverdino/aliyungo/slb](https://godoc.org/github.com/denverdino/aliyungo/slb) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/slb?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/slb)
  * SLS: [https://godoc.org/github.com/denverdino/aliyungo/sls](https://godoc.org/github.com/denverdino/aliyungo/sls) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/sls?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/sls)
  * SMS: [https://godoc.org/github.com/denverdino/aliyungo/sms](https://godoc.org/github.com/denverdino/aliyungo/sms) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/sms?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/sms)
  * STS: [https://godoc.org/github.com/denverdino/aliyungo/sts](https://godoc.org/github.com/denverdino/aliyungo/sts) [![GoDoc](https://godoc.org/github.com/denverdino/aliyungo/sts?status.svg)](https://godoc.org/github.com/denverdino/aliyungo/sts)

## Build and Install

go get:

```sh
go get github.com/denverdino/aliyungo
```

## Test ECS

Modify "ecs/config_test.go"

```sh
	TestAccessKeyId     = "MY_ACCESS_KEY_ID"
	TestAccessKeySecret = "MY_ACCESS_KEY_ID"
	TestInstanceId      = "MY_INSTANCE_ID"
	TestIAmRich         = false
```

* TestAccessKeyId: the Access Key Id
* TestAccessKeySecret: the Access Key Secret.
* TestInstanceId: the existing instance id for testing. It will be stopped and restarted during testing.
* TestIAmRich(Optional): If it is set to true, it will perform tests to create virtual machines and disks under your account. And you will pay the bill. :-)

Under "ecs" and run

```sh
go test
```

## Test OSS

Modify "oss/config_test.go"

```sh
	TestAccessKeyId     = "MY_ACCESS_KEY_ID"
	TestAccessKeySecret = "MY_ACCESS_KEY_ID"
	TestRegion          = oss.Beijing
	TestBucket          = "denverdino"
```

* TestAccessKeyId: the Access Key Id
* TestAccessKeySecret: the Access Key Secret.
* TestRegion: the region of OSS for testing
* TestBucket: the bucket name for testing

Under "oss" and run

```sh
go test
```

## Contributors

  * Li Yi (denverdino@gmail.com)
  * Boshi Lian (farmer1992@gmail.com)
  * Yu Zhou (oscarrr110@gmail.com)
  * Yufei Zhang
  * linuxlikerqq
  * Changhai Yan
  * Jizhong Jiang (jiangjizhong@gmail.com)
  * Kent Wang (pragkent@gmail.com)
  * ringtail
  * aiden0z (aiden0xz@gmail.com)
  * jimmycmh
  * menglingwei
  * mingang.he (dustgle@gmail.com)
  * Young Chen (chainone@gmail.com)
  * johnzeng
  * spacexnice (445436286@qq.com)
  * xiaoheihero
  * hmgle (dustgle@gmail.com)
  * jzwlqx (jiangjizhong@gmail.com)
  * Linhua Tan (toolchainX@gmail.com)
  * Plutonist (p@vecsight.com)
  * Bin Liu
  * wangyue
  * demonwy
  * yarous224
  * yufeizyf (xazyf9111@sina.cn)
  * keontang (ikeontang@gmail.com)
  * Cholerae Hu (me@cholerae.com)
  * Zach Bergh (berghzach@gmail.com)
  * Bingshen Wang
  * xiaozhu36
  * Russell (yufeiwu@gmail.com)
  * zhuzhih2017
  * cheyang
  * Hobo Chen
  * Shuwei Yin
  * Xujin Zheng (xujinzheng@gmail.com)
  * Dino Lai (dinos80152@gmail.com)

## License

This project is licensed under the Apache License, Version 2.0. See [LICENSE](https://github.com/denverdino/aliyungo/blob/master/LICENSE.txt) for the full license text.

## Related projects

  * Aliyun ECS driver for Docker Machine: [Pull request](https://github.com/docker/machine/pull/1182)

  * Aliyun OSS driver for Docker Registry V2: [Pull request](https://github.com/docker/distribution/pull/514)

## References

The GO API design of OSS refer the implementation from [https://github.com/AdRoll/goamz](https://github.com/AdRoll)
