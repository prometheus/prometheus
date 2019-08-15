
package endpoints

import (
	"encoding/json"
	"fmt"
	"sync"
)

const endpointsJson =`{
	"products": [
		{
			"code": "emr",
			"document_id": "28140",
			"location_service_code": "emr",
			"regional_endpoints": [
				{
					"region": "cn-qingdao",
					"endpoint": "emr.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "emr.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "emr.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "emr.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "emr.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "emr.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "emr.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "emr.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "emr.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "emr.ap-south-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "emr.us-east-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "emr.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "emr.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "emr.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "emr.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "emr.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "emr.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "emr.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "emr.aliyuncs.com"
				}
			],
			"global_endpoint": "emr.aliyuncs.com",
			"regional_endpoint_pattern": "emr.[RegionId].aliyuncs.com"
		},
		{
			"code": "petadata",
			"document_id": "",
			"location_service_code": "petadata",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "petadata.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "petadata.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "petadata.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "petadata.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "petadata.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "petadata.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "petadata.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "petadata.aliyuncs.com"
				}
			],
			"global_endpoint": "petadata.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dbs",
			"document_id": "",
			"location_service_code": "dbs",
			"regional_endpoints": [
				{
					"region": "cn-shenzhen",
					"endpoint": "dbs-api.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "dbs-api.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "dbs-api.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "dbs-api.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "dbs-api.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "dbs-api.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "dbs-api.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "dbs-api.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "alidnsgtm",
			"document_id": "",
			"location_service_code": "alidnsgtm",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "alidns.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "alidns.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "elasticsearch",
			"document_id": "",
			"location_service_code": "elasticsearch",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "elasticsearch.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "elasticsearch.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "elasticsearch.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "elasticsearch.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "elasticsearch.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "elasticsearch.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "elasticsearch.us-west-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "elasticsearch.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "elasticsearch.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "elasticsearch.eu-central-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "elasticsearch.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "elasticsearch.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "elasticsearch.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "elasticsearch.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "elasticsearch.ap-northeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "baas",
			"document_id": "",
			"location_service_code": "baas",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "baas.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "baas.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "baas.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "baas.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "baas.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "baas.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cr",
			"document_id": "60716",
			"location_service_code": "cr",
			"regional_endpoints": null,
			"global_endpoint": "cr.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cloudap",
			"document_id": "",
			"location_service_code": "cloudap",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "cloudwf.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "imagesearch",
			"document_id": "",
			"location_service_code": "imagesearch",
			"regional_endpoints": [
				{
					"region": "ap-southeast-2",
					"endpoint": "imagesearch.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "imagesearch.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "imagesearch.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "imagesearch.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "pts",
			"document_id": "",
			"location_service_code": "pts",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "pts.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ehs",
			"document_id": "",
			"location_service_code": "ehs",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "ehpc.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "ehpc.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "ehpc.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "ehpc.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "ehpc.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "ehpc.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "ehpc.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ehpc.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ehpc.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "ehpc.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "ehpc.ap-southeast-2.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "polardb",
			"document_id": "58764",
			"location_service_code": "polardb",
			"regional_endpoints": [
				{
					"region": "ap-south-1",
					"endpoint": "polardb.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "polardb.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "polardb.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "polardb.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "polardb.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "polardb.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "polardb.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "polardb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "polardb.ap-southeast-5.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "polardb.aliyuncs.com"
		},
		{
			"code": "r-kvstore",
			"document_id": "60831",
			"location_service_code": "redisa",
			"regional_endpoints": [
				{
					"region": "cn-shenzhen",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "r-kvstore.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "r-kvstore.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "r-kvstore.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "r-kvstore.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "r-kvstore.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "r-kvstore.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "r-kvstore.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "r-kvstore.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "r-kvstore.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "r-kvstore.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "r-kvstore.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "r-kvstore.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "r-kvstore.ap-southeast-5.aliyuncs.com"
				}
			],
			"global_endpoint": "r-kvstore.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "xianzhi",
			"document_id": "",
			"location_service_code": "xianzhi",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "xianzhi.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "pcdn",
			"document_id": "",
			"location_service_code": "pcdn",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "pcdn.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cdn",
			"document_id": "27148",
			"location_service_code": "cdn",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "cdn.aliyuncs.com"
				}
			],
			"global_endpoint": "cdn.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cloudauth",
			"document_id": "60687",
			"location_service_code": "cloudauth",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "cloudauth.aliyuncs.com"
				}
			],
			"global_endpoint": "cloudauth.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "nas",
			"document_id": "62598",
			"location_service_code": "nas",
			"regional_endpoints": [
				{
					"region": "ap-southeast-2",
					"endpoint": "nas.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "nas.ap-south-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "nas.eu-central-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "nas.us-west-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "nas.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "nas.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "nas.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "nas.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "nas.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "nas.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "nas.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "nas.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "nas.us-east-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "nas.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "nas.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "nas.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "nas.ap-southeast-3.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "alidns",
			"document_id": "29739",
			"location_service_code": "alidns",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "alidns.aliyuncs.com"
				}
			],
			"global_endpoint": "alidns.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dts",
			"document_id": "",
			"location_service_code": "dts",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "dts.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "dts.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "emas",
			"document_id": "",
			"location_service_code": "emas",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "mhub.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "mhub.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dysmsapi",
			"document_id": "",
			"location_service_code": "dysmsapi",
			"regional_endpoints": [
				{
					"region": "ap-southeast-3",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "cn-chengdu",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "dysmsapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "dysmsapi.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "dysmsapi.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cloudwf",
			"document_id": "58111",
			"location_service_code": "cloudwf",
			"regional_endpoints": null,
			"global_endpoint": "cloudwf.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "fc",
			"document_id": "",
			"location_service_code": "fc",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "cn-beijing.fc.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "ap-southeast-2.fc.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "cn-huhehaote.fc.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "cn-shanghai.fc.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "cn-hangzhou.fc.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "cn-shenzhen.fc.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "saf",
			"document_id": "",
			"location_service_code": "saf",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "saf.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "saf.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "saf.cn-shenzhen.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "rds",
			"document_id": "26223",
			"location_service_code": "rds",
			"regional_endpoints": [
				{
					"region": "ap-northeast-1",
					"endpoint": "rds.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "rds.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "rds.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "rds.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "rds.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "rds.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "rds.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "rds.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "rds.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "rds.me-east-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "rds.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "rds.aliyuncs.com"
				}
			],
			"global_endpoint": "rds.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "vpc",
			"document_id": "34962",
			"location_service_code": "vpc",
			"regional_endpoints": [
				{
					"region": "ap-south-1",
					"endpoint": "vpc.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "vpc.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "vpc.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "vpc.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "vpc.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "vpc.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "vpc.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "vpc.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "vpc.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "vpc.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "vpc.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "vpc.aliyuncs.com"
				}
			],
			"global_endpoint": "vpc.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "gpdb",
			"document_id": "",
			"location_service_code": "gpdb",
			"regional_endpoints": [
				{
					"region": "ap-southeast-3",
					"endpoint": "gpdb.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "gpdb.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "gpdb.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "gpdb.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "gpdb.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "gpdb.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "gpdb.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "gpdb.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "gpdb.eu-west-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "gpdb.ap-south-1.aliyuncs.com"
				}
			],
			"global_endpoint": "gpdb.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "yunmarket",
			"document_id": "",
			"location_service_code": "yunmarket",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "market.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "pvtz",
			"document_id": "",
			"location_service_code": "pvtz",
			"regional_endpoints": [
				{
					"region": "ap-southeast-1",
					"endpoint": "pvtz.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "pvtz.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "oss",
			"document_id": "",
			"location_service_code": "oss",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "oss-cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "oss-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "oss-cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "oss-cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "oss-cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "oss-ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "oss-us-west-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "oss-cn-qingdao.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "foas",
			"document_id": "",
			"location_service_code": "foas",
			"regional_endpoints": [
				{
					"region": "cn-qingdao",
					"endpoint": "foas.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "foas.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "foas.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "foas.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "foas.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "foas.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "foas.ap-northeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ddos",
			"document_id": "",
			"location_service_code": "ddos",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ddospro.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "ddospro.cn-hongkong.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cbn",
			"document_id": "",
			"location_service_code": "cbn",
			"regional_endpoints": [
				{
					"region": "ap-southeast-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "cbn.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "cbn.aliyuncs.com"
				}
			],
			"global_endpoint": "cbn.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "nlp",
			"document_id": "",
			"location_service_code": "nlp",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "nlp.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hsm",
			"document_id": "",
			"location_service_code": "hsm",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "hsm.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "hsm.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "hsm.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "hsm.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "hsm.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "hsm.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ons",
			"document_id": "44416",
			"location_service_code": "ons",
			"regional_endpoints": [
				{
					"region": "ap-southeast-1",
					"endpoint": "ons.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "ons.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "ons.us-east-1.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "ons.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "ons.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "ons.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ons.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "ons.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "ons.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "ons.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "ons.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "ons.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "ons.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "ons.eu-central-1.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "ons.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "ons.us-west-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "ons.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ons.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "kms",
			"document_id": "",
			"location_service_code": "kms",
			"regional_endpoints": [
				{
					"region": "cn-hongkong",
					"endpoint": "kms.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "kms.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "kms.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "kms.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "kms.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "kms.us-east-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "kms.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "kms.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "kms.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "kms.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "kms.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "kms.me-east-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "kms.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "kms.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "kms.us-west-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "kms.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "kms.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "kms.eu-central-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "kms.ap-northeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cps",
			"document_id": "",
			"location_service_code": "cps",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "cloudpush.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ensdisk",
			"document_id": "",
			"location_service_code": "ensdisk",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ens.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cloudapi",
			"document_id": "43590",
			"location_service_code": "apigateway",
			"regional_endpoints": [
				{
					"region": "ap-southeast-2",
					"endpoint": "apigateway.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "apigateway.ap-south-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "apigateway.us-east-1.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "apigateway.me-east-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "apigateway.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "apigateway.cn-beijing.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "apigateway.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "apigateway.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "apigateway.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "apigateway.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "apigateway.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "apigateway.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "apigateway.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "apigateway.us-west-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "apigateway.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "apigateway.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "apigateway.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "apigateway.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "apigateway.cn-hongkong.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "apigateway.[RegionId].aliyuncs.com"
		},
		{
			"code": "eci",
			"document_id": "",
			"location_service_code": "eci",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "eci.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "eci.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "eci.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "eci.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "eci.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "eci.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "onsvip",
			"document_id": "",
			"location_service_code": "onsvip",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "ons.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ons.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ons.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "ons.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "ons.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "ons.cn-qingdao.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "linkwan",
			"document_id": "",
			"location_service_code": "linkwan",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "linkwan.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "linkwan.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ddosdip",
			"document_id": "",
			"location_service_code": "ddosdip",
			"regional_endpoints": [
				{
					"region": "ap-southeast-1",
					"endpoint": "ddosdip.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "batchcompute",
			"document_id": "44717",
			"location_service_code": "batchcompute",
			"regional_endpoints": [
				{
					"region": "us-west-1",
					"endpoint": "batchcompute.us-west-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "batchcompute.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "batchcompute.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "batchcompute.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "batchcompute.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "batchcompute.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "batchcompute.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "batchcompute.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "batchcompute.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "batchcompute.[RegionId].aliyuncs.com"
		},
		{
			"code": "aegis",
			"document_id": "28449",
			"location_service_code": "vipaegis",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "aegis.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "aegis.ap-southeast-3.aliyuncs.com"
				}
			],
			"global_endpoint": "aegis.cn-hangzhou.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "arms",
			"document_id": "42924",
			"location_service_code": "arms",
			"regional_endpoints": [
				{
					"region": "cn-hongkong",
					"endpoint": "arms.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "arms.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "arms.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "arms.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "arms.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "arms.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "arms.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "arms.[RegionId].aliyuncs.com"
		},
		{
			"code": "live",
			"document_id": "48207",
			"location_service_code": "live",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "live.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "live.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "live.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "live.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "live.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "live.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "live.aliyuncs.com"
				}
			],
			"global_endpoint": "live.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "alimt",
			"document_id": "",
			"location_service_code": "alimt",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "mt.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "actiontrail",
			"document_id": "",
			"location_service_code": "actiontrail",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "actiontrail.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "actiontrail.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "actiontrail.us-east-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "actiontrail.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "actiontrail.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "actiontrail.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "actiontrail.ap-south-1.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "actiontrail.me-east-1.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "actiontrail.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "actiontrail.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "actiontrail.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "actiontrail.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "actiontrail.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "actiontrail.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "actiontrail.us-west-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "actiontrail.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "actiontrail.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "actiontrail.cn-beijing.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "actiontrail.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "smartag",
			"document_id": "",
			"location_service_code": "smartag",
			"regional_endpoints": [
				{
					"region": "ap-southeast-3",
					"endpoint": "smartag.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "smartag.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "smartag.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "smartag.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "smartag.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "smartag.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "smartag.ap-southeast-2.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "vod",
			"document_id": "60574",
			"location_service_code": "vod",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "vod.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "vod.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "vod.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "vod.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "vod.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "vod.eu-central-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "domain",
			"document_id": "42875",
			"location_service_code": "domain",
			"regional_endpoints": [
				{
					"region": "ap-southeast-1",
					"endpoint": "domain-intl.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "domain.aliyuncs.com"
				}
			],
			"global_endpoint": "domain.aliyuncs.com",
			"regional_endpoint_pattern": "domain.aliyuncs.com"
		},
		{
			"code": "ros",
			"document_id": "28899",
			"location_service_code": "ros",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ros.aliyuncs.com"
				}
			],
			"global_endpoint": "ros.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cloudphoto",
			"document_id": "59902",
			"location_service_code": "cloudphoto",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "cloudphoto.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "cloudphoto.[RegionId].aliyuncs.com"
		},
		{
			"code": "rtc",
			"document_id": "",
			"location_service_code": "rtc",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "rtc.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "odpsmayi",
			"document_id": "",
			"location_service_code": "odpsmayi",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "bsb.cloud.alipay.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "bsb.cloud.alipay.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ims",
			"document_id": "",
			"location_service_code": "ims",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ims.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "csb",
			"document_id": "64837",
			"location_service_code": "csb",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "csb.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "csb.cn-beijing.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "csb.[RegionId].aliyuncs.com"
		},
		{
			"code": "cds",
			"document_id": "62887",
			"location_service_code": "codepipeline",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "cds.cn-beijing.aliyuncs.com"
				}
			],
			"global_endpoint": "cds.cn-beijing.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ddosbgp",
			"document_id": "",
			"location_service_code": "ddosbgp",
			"regional_endpoints": [
				{
					"region": "cn-huhehaote",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "ddosbgp.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "ddosbgp.us-west-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ddosbgp.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dybaseapi",
			"document_id": "",
			"location_service_code": "dybaseapi",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "cn-chengdu",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "dybaseapi.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "dybaseapi.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ecs",
			"document_id": "25484",
			"location_service_code": "ecs",
			"regional_endpoints": [
				{
					"region": "cn-huhehaote",
					"endpoint": "ecs.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "ecs.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "ecs.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "ecs.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "ecs.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "ecs.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "ecs.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "ecs.eu-west-1.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "ecs.me-east-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "ecs.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "ecs-cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "ecs-cn-hangzhou.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ccc",
			"document_id": "63027",
			"location_service_code": "ccc",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ccc.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ccc.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "ccc.[RegionId].aliyuncs.com"
		},
		{
			"code": "cs",
			"document_id": "26043",
			"location_service_code": "cs",
			"regional_endpoints": null,
			"global_endpoint": "cs.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "drdspre",
			"document_id": "",
			"location_service_code": "drdspre",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "drds.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "drds.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "drds.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "drds.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "drds.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "drds.cn-qingdao.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "drds.cn-beijing.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dcdn",
			"document_id": "",
			"location_service_code": "dcdn",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "dcdn.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "dcdn.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "linkedmall",
			"document_id": "",
			"location_service_code": "linkedmall",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "linkedmall.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "linkedmall.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "trademark",
			"document_id": "",
			"location_service_code": "trademark",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "trademark.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "openanalytics",
			"document_id": "",
			"location_service_code": "openanalytics",
			"regional_endpoints": [
				{
					"region": "cn-shenzhen",
					"endpoint": "openanalytics.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "openanalytics.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "openanalytics.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "openanalytics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "openanalytics.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "openanalytics.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "openanalytics.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "openanalytics.cn-zhangjiakou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "sts",
			"document_id": "28756",
			"location_service_code": "sts",
			"regional_endpoints": null,
			"global_endpoint": "sts.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "waf",
			"document_id": "62847",
			"location_service_code": "waf",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "wafopenapi.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ots",
			"document_id": "",
			"location_service_code": "ots",
			"regional_endpoints": [
				{
					"region": "me-east-1",
					"endpoint": "ots.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "ots.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "ots.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "ots.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "ots.cn-beijing.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "ots.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "ots.us-west-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "ots.us-east-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "ots.ap-south-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "ots.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "ots.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ots.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "ots.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ots.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "ots.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "ots.ap-southeast-3.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cloudfirewall",
			"document_id": "",
			"location_service_code": "cloudfirewall",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "cloudfw.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dm",
			"document_id": "29434",
			"location_service_code": "dm",
			"regional_endpoints": [
				{
					"region": "ap-southeast-2",
					"endpoint": "dm.ap-southeast-2.aliyuncs.com"
				}
			],
			"global_endpoint": "dm.aliyuncs.com",
			"regional_endpoint_pattern": "dm.[RegionId].aliyuncs.com"
		},
		{
			"code": "oas",
			"document_id": "",
			"location_service_code": "oas",
			"regional_endpoints": [
				{
					"region": "cn-shenzhen",
					"endpoint": "cn-shenzhen.oas.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "cn-beijing.oas.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "cn-hangzhou.oas.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ddoscoo",
			"document_id": "",
			"location_service_code": "ddoscoo",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ddoscoo.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "jaq",
			"document_id": "35037",
			"location_service_code": "jaq",
			"regional_endpoints": null,
			"global_endpoint": "jaq.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "iovcc",
			"document_id": "",
			"location_service_code": "iovcc",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "iovcc.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "sas-api",
			"document_id": "28498",
			"location_service_code": "sas",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "sas.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "chatbot",
			"document_id": "60760",
			"location_service_code": "beebot",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "chatbot.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "chatbot.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "chatbot.[RegionId].aliyuncs.com"
		},
		{
			"code": "airec",
			"document_id": "",
			"location_service_code": "airec",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "airec.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "airec.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "airec.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "dmsenterprise",
			"document_id": "",
			"location_service_code": "dmsenterprise",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "dms-enterprise.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "dms-enterprise.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "dms-enterprise.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "dms-enterprise.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "dms-enterprise.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "dms-enterprise.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ivision",
			"document_id": "",
			"location_service_code": "ivision",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ivision.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "ivision.cn-beijing.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "odpsplusmayi",
			"document_id": "",
			"location_service_code": "odpsplusmayi",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "bsb.cloud.alipay.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "bsb.cloud.alipay.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "bsb.cloud.alipay.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "gameshield",
			"document_id": "",
			"location_service_code": "gameshield",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "gameshield.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "gameshield.cn-zhangjiakou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "scdn",
			"document_id": "",
			"location_service_code": "scdn",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "scdn.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hitsdb",
			"document_id": "",
			"location_service_code": "hitsdb",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "hitsdb.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hdm",
			"document_id": "",
			"location_service_code": "hdm",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "hdm-api.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "slb",
			"document_id": "27565",
			"location_service_code": "slb",
			"regional_endpoints": [
				{
					"region": "cn-shenzhen",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "slb.eu-west-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "slb.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "slb.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "slb.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "slb.ap-south-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "slb.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "slb.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "slb.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "slb.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "slb.me-east-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "slb.cn-zhangjiakou.aliyuncs.com"
				}
			],
			"global_endpoint": "slb.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "green",
			"document_id": "28427",
			"location_service_code": "green",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "green.cn-beijing.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "green.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "green.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "green.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "green.us-west-1.aliyuncs.com"
				}
			],
			"global_endpoint": "green.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cccvn",
			"document_id": "",
			"location_service_code": "cccvn",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "voicenavigator.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ddosrewards",
			"document_id": "",
			"location_service_code": "ddosrewards",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ddosright.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "iot",
			"document_id": "30557",
			"location_service_code": "iot",
			"regional_endpoints": [
				{
					"region": "us-east-1",
					"endpoint": "iot.us-east-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "iot.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "iot.us-west-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "iot.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "iot.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "iot.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": "iot.[RegionId].aliyuncs.com"
		},
		{
			"code": "bssopenapi",
			"document_id": "",
			"location_service_code": "bssopenapi",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "business.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "business.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "sca",
			"document_id": "",
			"location_service_code": "sca",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "qualitycheck.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "luban",
			"document_id": "",
			"location_service_code": "luban",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "luban.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "luban.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "drdspost",
			"document_id": "",
			"location_service_code": "drdspost",
			"regional_endpoints": [
				{
					"region": "cn-shanghai",
					"endpoint": "drds.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "drds.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "drds.ap-southeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "drds",
			"document_id": "51111",
			"location_service_code": "drds",
			"regional_endpoints": [
				{
					"region": "ap-southeast-1",
					"endpoint": "drds.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "drds.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "drds.aliyuncs.com",
			"regional_endpoint_pattern": "drds.aliyuncs.com"
		},
		{
			"code": "httpdns",
			"document_id": "52679",
			"location_service_code": "httpdns",
			"regional_endpoints": null,
			"global_endpoint": "httpdns-api.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cas",
			"document_id": "",
			"location_service_code": "cas",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "cas.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "cas.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "cas.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "cas.eu-central-1.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "cas.me-east-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hpc",
			"document_id": "35201",
			"location_service_code": "hpc",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "hpc.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "hpc.aliyuncs.com"
				}
			],
			"global_endpoint": "hpc.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ddosbasic",
			"document_id": "",
			"location_service_code": "ddosbasic",
			"regional_endpoints": [
				{
					"region": "ap-south-1",
					"endpoint": "antiddos-openapi.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "antiddos-openapi.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "antiddos-openapi.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "antiddos-openapi.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "antiddos-openapi.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "antiddos-openapi.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "antiddos-openapi.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "antiddos-openapi.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "antiddos-openapi.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "antiddos.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "antiddos-openapi.ap-northeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "antiddos.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "clouddesktop",
			"document_id": "",
			"location_service_code": "clouddesktop",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "clouddesktop.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "clouddesktop.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "clouddesktop.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "clouddesktop.cn-shenzhen.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "uis",
			"document_id": "",
			"location_service_code": "uis",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "uis.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "imm",
			"document_id": "",
			"location_service_code": "imm",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "imm.cn-beijing.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "imm.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "imm.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "imm.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "imm.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "imm.cn-shenzhen.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ens",
			"document_id": "",
			"location_service_code": "ens",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ens.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ram",
			"document_id": "28672",
			"location_service_code": "ram",
			"regional_endpoints": null,
			"global_endpoint": "ram.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hcs_mgw",
			"document_id": "",
			"location_service_code": "hcs_mgw",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "mgw.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "mgw.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "mgw.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "itaas",
			"document_id": "55759",
			"location_service_code": "itaas",
			"regional_endpoints": null,
			"global_endpoint": "itaas.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "qualitycheck",
			"document_id": "50807",
			"location_service_code": "qualitycheck",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "qualitycheck.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "alikafka",
			"document_id": "",
			"location_service_code": "alikafka",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "alikafka.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "alikafka.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "alikafka.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "alikafka.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "alikafka.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "alikafka.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "alikafka.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "alikafka.cn-qingdao.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "faas",
			"document_id": "",
			"location_service_code": "faas",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "faas.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "faas.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "faas.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "faas.cn-beijing.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "alidfs",
			"document_id": "",
			"location_service_code": "alidfs",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "dfs.cn-beijing.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "dfs.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "cms",
			"document_id": "28615",
			"location_service_code": "cms",
			"regional_endpoints": [
				{
					"region": "ap-southeast-3",
					"endpoint": "metrics.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "metrics.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "metrics.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "metrics.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "metrics.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "metrics.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "metrics.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "metrics.cn-hangzhou.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "domain-intl",
			"document_id": "",
			"location_service_code": "domain-intl",
			"regional_endpoints": null,
			"global_endpoint": "domain-intl.aliyuncs.com",
			"regional_endpoint_pattern": "domain-intl.aliyuncs.com"
		},
		{
			"code": "kvstore",
			"document_id": "",
			"location_service_code": "kvstore",
			"regional_endpoints": [
				{
					"region": "ap-northeast-1",
					"endpoint": "r-kvstore.ap-northeast-1.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ccs",
			"document_id": "",
			"location_service_code": "ccs",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "ccs.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "ess",
			"document_id": "25925",
			"location_service_code": "ess",
			"regional_endpoints": [
				{
					"region": "cn-huhehaote",
					"endpoint": "ess.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "ess.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "ess.eu-west-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "ess.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "ess.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "ess.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "ess.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "ess.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "ess.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "ess.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "ess.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "ess.aliyuncs.com"
				}
			],
			"global_endpoint": "ess.aliyuncs.com",
			"regional_endpoint_pattern": "ess.[RegionId].aliyuncs.com"
		},
		{
			"code": "dds",
			"document_id": "61715",
			"location_service_code": "dds",
			"regional_endpoints": [
				{
					"region": "me-east-1",
					"endpoint": "mongodb.me-east-1.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "mongodb.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "mongodb.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "mongodb.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "mongodb.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "mongodb.eu-west-1.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "cn-huhehaote",
					"endpoint": "mongodb.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "mongodb.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "mongodb.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "mongodb.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "mongodb.ap-south-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "mongodb.aliyuncs.com"
				}
			],
			"global_endpoint": "mongodb.aliyuncs.com",
			"regional_endpoint_pattern": "mongodb.[RegionId].aliyuncs.com"
		},
		{
			"code": "mts",
			"document_id": "29212",
			"location_service_code": "mts",
			"regional_endpoints": [
				{
					"region": "cn-beijing",
					"endpoint": "mts.cn-beijing.aliyuncs.com"
				},
				{
					"region": "ap-northeast-1",
					"endpoint": "mts.ap-northeast-1.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "mts.cn-hongkong.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "mts.cn-shenzhen.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "mts.cn-zhangjiakou.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "mts.ap-south-1.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "mts.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "mts.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "mts.us-west-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "mts.eu-central-1.aliyuncs.com"
				},
				{
					"region": "eu-west-1",
					"endpoint": "mts.eu-west-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "mts.cn-hangzhou.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "push",
			"document_id": "30074",
			"location_service_code": "push",
			"regional_endpoints": null,
			"global_endpoint": "cloudpush.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hcs_sgw",
			"document_id": "",
			"location_service_code": "hcs_sgw",
			"regional_endpoints": [
				{
					"region": "eu-central-1",
					"endpoint": "sgw.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-zhangjiakou",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "sgw.ap-southeast-1.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-hongkong",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "sgw.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "sgw.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "hbase",
			"document_id": "",
			"location_service_code": "hbase",
			"regional_endpoints": [
				{
					"region": "cn-huhehaote",
					"endpoint": "hbase.cn-huhehaote.aliyuncs.com"
				},
				{
					"region": "ap-south-1",
					"endpoint": "hbase.ap-south-1.aliyuncs.com"
				},
				{
					"region": "us-west-1",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "me-east-1",
					"endpoint": "hbase.me-east-1.aliyuncs.com"
				},
				{
					"region": "eu-central-1",
					"endpoint": "hbase.eu-central-1.aliyuncs.com"
				},
				{
					"region": "cn-qingdao",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "cn-shenzhen",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "ap-southeast-2",
					"endpoint": "hbase.ap-southeast-2.aliyuncs.com"
				},
				{
					"region": "ap-southeast-3",
					"endpoint": "hbase.ap-southeast-3.aliyuncs.com"
				},
				{
					"region": "cn-hangzhou",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "us-east-1",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "ap-southeast-5",
					"endpoint": "hbase.ap-southeast-5.aliyuncs.com"
				},
				{
					"region": "cn-beijing",
					"endpoint": "hbase.aliyuncs.com"
				},
				{
					"region": "ap-southeast-1",
					"endpoint": "hbase.aliyuncs.com"
				}
			],
			"global_endpoint": "hbase.aliyuncs.com",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "bastionhost",
			"document_id": "",
			"location_service_code": "bastionhost",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "yundun-bastionhost.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		},
		{
			"code": "vs",
			"document_id": "",
			"location_service_code": "vs",
			"regional_endpoints": [
				{
					"region": "cn-hangzhou",
					"endpoint": "vs.cn-hangzhou.aliyuncs.com"
				},
				{
					"region": "cn-shanghai",
					"endpoint": "vs.cn-shanghai.aliyuncs.com"
				}
			],
			"global_endpoint": "",
			"regional_endpoint_pattern": ""
		}
	]
}`
var initOnce sync.Once
var data interface{}

func getEndpointConfigData() interface{} {
	initOnce.Do(func() {
		err := json.Unmarshal([]byte(endpointsJson), &data)
		if err != nil {
			panic(fmt.Sprintf("init endpoint config data failed. %s", err))
		}
	})
	return data
}
