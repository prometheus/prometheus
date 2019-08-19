## 阿里云ECS服务发现配置

### 样例prometheus.yaml

```yaml
global:
  scrape_interval: 15s
  scrape_timeout: 10s
  evaluation_interval: 30s
scrape_configs:
- job_name: _aliyun-prom/ecs-sd
  honor_timestamps: true
  scrape_interval: 30s
  scrape_timeout: 10s
  metrics_path: /metrics
  scheme: https
  aliyun_sd_configs:
    - port:  8888                                 # 服务发现后的prometheus抓取采集点port
      user_id: <aliyun userId>                    # Aliyun 用户身份表示id userId, 填写会为discovery target带上__meta_ecs_user_id的label，可不填写
      refresh_interval: 30s
      region_id: cn-hangzhou                      # 设置获取ECS的regionId
      access_key: <aliyun ak>                     # Aliyun 鉴权字段 AK
      access_key_secret: <aliyun sk>              # Aliyun 鉴权字段 SK
      limit: 40                                   # 从接口取到的最大实例个数限制，default is 100

# relabel为可选配置，用于手动筛选实例
  relabel_configs:

#   1. 手动设置使用ECS的哪种IP
#   默认ECS会按 经典网络公网IP > 经典网络内网IP > VPC网络公网IP > VPC网络内网IP 的顺序查找并赋予此ECS的采集IP，此时的采集点port为aliyun_sd_configs.port设置
#   用户可用过一下relabel设置，手动设置ECS的采集IP
    - source_labels: [__meta_ecs_public_ip]       # 经典网络公网ip __meta_ecs_public_ip
#    - source_labels: [__meta_ecs_inner_ip]       # 经典网络内网ip __meta_ecs_inner_ip
#    - source_labels: [__meta_ecs_eip]            # VPC网络 公网ip __meta_ecs_eip
#    - source_labels: [__meta_ecs_private_ip]     # VPC网络 内网ip __meta_ecs_private_ip
      regex: (.*)
      target_label: __address__
      replacement: $1:<port>                      # 注意此处为手动设置relabel时的采集port

#   2. 按ECS属性过滤 keep为只保留此条件筛选到的target，drop为过滤掉此条件筛选到的target
#   __meta_ecs_instance_id          实例id
#   __meta_ecs_region_id            实例regionId 注意配置中aliyun_sd_configs.region_id决定了获取的ECS的regionId
#   __meta_ecs_status               实例状态 Running：运行中、Starting：启动中、Stopping：停止中、Stopped：已停止
#   __meta_ecs_zone_id              实例区域id
#   __meta_ecs_network_type         实例网络类型  classic：经典网络、vpc：VPC
#   __meta_ecs_tag_<TagKey>         实例tag TagKey为tag的名
    - source_labels:  ["__meta_ecs_instance_id"]
      regex: ".+"       # or other value regex
      action: keep      # keep / drop
```
