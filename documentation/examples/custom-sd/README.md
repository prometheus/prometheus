# Custom SD
Custom SD is available via the file_sd adapter, which can be used to access SD mechanisms that are not 
included in official Prometheus releases. The sd adapter outputs a file that can be passed via your `prometheus.yml` 
as a `file_sd` file. This will allow you to pass your targets to Prometheus for scraping without having 
to use a static config.

# Example
This directory (`documentation/examples/custom-sd`) contains an example custom service discovery implementation
as well as a file_sd adapter usage. `adapter-usage` contains the `Discoverer` implementation for a basic Consul
service discovery mechanism. It simply queries Consul for all it's known services (except Consul itself), 
and sends them along with all the other service data as labels as a TargetGroup.
The `adapter` directory contains the adapter code you will want to import and pass your `Discoverer` 
implementation to.

# Usage
To use file_sd adapter you must implement a `Discoverer`. In `adapter-usage/main.go` replace the example 
SD config and create an instance of your SD implementation to pass to the `Adapter`'s `NewAdapter`. See the
`Note:` comments for the structs and `Run` function.