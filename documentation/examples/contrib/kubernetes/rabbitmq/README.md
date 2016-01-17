# RabbitMQ Scraping

This is an example on how to setup RabbitMQ so prometheus can scrap data from it.
It uses a third party [RabbitMQ exporter](https://github.com/kbudde/rabbitmq_exporter).

Since the [RabbitMQ exporter](https://github.com/kbudde/rabbitmq_exporter) needs to
connect on RabbitMQ management API to scrap data, and it defaults to localhost, it is
easier to simply embed the **kbudde/rabbitmq-exporter** on the same pod as RabbitMQ,
this way they share the same network.

With this pod running you will have the exporter scraping data, but prometheus have not
yet found the exporter and is not scraping data from it.

For more details on how to use kubernetes service discovery take a look on the 
[documentation](http://prometheus.io/docs/operating/configuration/#kubernetes-sd-configurations-kubernetes_sd_config)
and on the [available examples](./documentation/examples).

After you got Kubernetes service discovery up and running you just need to advertise that RabbitMQ
is exposing metrics. To do that you need to define a service that:

* Exposes the exporter port
* Add the annotation: prometheus.io/scrape: "true"
* Add the annotation: prometheus.io/port: "9090"

And you should be able to see your RabbitMQ exporter being scrapped on prometheus status page.
Since the ip that will be scrapped will be the pod endpoint it is important that the node
where prometheus is running have access to the Kubernetes overlay network
(flannel, weave, aws, or any of the other options that Kubernetes gives to you).
