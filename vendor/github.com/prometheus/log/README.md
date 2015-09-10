# Prometheus Logging Library

Standard logging library for Go-based Prometheus components.

This library wraps
[https://github.com/Sirupsen/logrus](https://github.com/Sirupsen/logrus) in
order to add line:file annotations to log lines, as well as to provide common
command-line flags for Prometheus components using it.
