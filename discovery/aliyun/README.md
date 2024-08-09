## Aliyun ECS service discovery config

### prometheus-example.yaml

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
  ecs_sd_configs:
    - port:  8888                                 # Prometheus crawling collection point port after service discovery
      user_id: <aliyun userId>                    # Aliyun user id, optional, filling this in will add the label __meta_ecs_user_id to the discovery target.
      refresh_interval: 30s
      region_id: cn-hangzhou                      # the region of ecs
      access_key: <aliyun ak>                     # Aliyun AccessKeyID
      access_key_secret: <aliyun sk>              # Aliyun AccessKeySecret
      tag_filters:                                # Aliyun ECS tag filter
       - key: 'testK'
         values: ['*', 'test1*']
       - key: 'testM'
         values: ['test2*']
#     limit: 100                                  # The maximum number of instances to query, default value is 100, get all instances when less than zero.

# Optional, for manual screening of instances
  relabel_configs:
    
# 1. Manually set the IP address of the ECS
#    By default, the ECS will search and assign the collection IP to this ECS in the order of 
#    classic network public IP > classic network private IP > VPC network public IP > VPC network private IP.
#    The collection point port is set by aliyun_sd_configs.port.
    - source_labels: [__meta_ecs_public_ip]      # classic network public IP
#   - source_labels: [__meta_ecs_inner_ip]       # classic network private IP
#   - source_labels: [__meta_ecs_eip]            # VPC network public IP
#   - source_labels: [__meta_ecs_private_ip]     # VPC network private IP
      regex: (.*)
      target_label: __address__
      replacement: $1:<port>                      # Set the collection port when relabeling

# 2. Filter by ECS attribute 
#    [keep] means only keep the target filtered by this condition, 
#    [drop] means filter out the target filtered by this condition.
#     __meta_ecs_instance_id          instance id
#     __meta_ecs_region_id            instance region, Note that aliyun_sd_configs.region_id in the configuration determines the region of the ECS query
#     __meta_ecs_status               instance status, including Running, Starting, Stopping and Stopped
#     __meta_ecs_zone_id              instance zone id
#     __meta_ecs_network_type         instance network type, including  classic and vpc
#     __meta_ecs_tag_<TagKey>         instance tag
#   - source_labels:  ["__meta_ecs_instance_id"]
#     regex: ".+"       # or other value regex
#     action: keep      # keep / drop
```
