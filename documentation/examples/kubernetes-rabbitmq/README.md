# RabbitMQ Scraping

This is an example on how to setup RabbitMQ so Prometheus can scrape data from it.
It uses a third party [RabbitMQ exporter](https://github.com/kbudde/rabbitmq_exporter).

Since the [RabbitMQ exporter](https://github.com/kbudde/rabbitmq_exporter) needs to
scrape the RabbitMQ management API to scrape data, and it defaults to localhost, it is
easier to simply embed the **kbudde/rabbitmq-exporter** on the same pod as RabbitMQ,
this way they share the same network.

With this pod running you will have the exporter scraping data, but Prometheus has not
yet found the exporter and is not scraping data from it.

For more details on how to use Kubernetes service discovery take a look at the 
[documentation](https://prometheus.io/docs/operating/configuration/#kubernetes-sd-configurations-kubernetes_sd_config)
and at the [available examples](./../).

After you have gotten Kubernetes service discovery up and running, add the following scrape_configs to prometheus.yml.

```
scrape_configs:
- job_name: 'RabbitMQ'
  kubernetes_sd_configs:
  - role: pod
  relabel_configs:
  - source_labels:
    - __meta_kubernetes_pod_label_app
    regex: rabbitmq
    action: keep
```

And you should be able to see your RabbitMQ exporter being scraped on the Prometheus status page.
Since the IP that will be scraped will be the pod endpoint it is important that the node
where Prometheus is running has access to the Kubernetes overlay network
(flannel, Weave, AWS, or any of the other options that Kubernetes gives to you).
