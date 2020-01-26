# shipping

This example demonstrates a more real-world application consisting of multiple services.

## Description

The implementation is based on the container shipping domain from the [Domain Driven Design](http://www.amazon.com/Domain-Driven-Design-Tackling-Complexity-Software/dp/0321125215) book by Eric Evans, which was [originally](http://dddsample.sourceforge.net/) implemented in Java but has since been ported to Go. This example is a somewhat stripped down version to demonstrate the use of Go kit. The [original Go application](https://github.com/marcusolsson/goddd) is maintained separately and accompanied by an [AngularJS application](https://github.com/marcusolsson/dddelivery-angularjs) as well as a mock [routing service](https://github.com/marcusolsson/pathfinder). 

### Organization

The application consists of three application services, `booking`, `handling` and `tracking`. Each of these is an individual Go kit service as seen in previous examples. 

- __booking__ - used by the shipping company to book and route cargos.
- __handling__ - used by our staff around the world to register whenever the cargo has been received, loaded etc.
- __tracking__ - used by the customer to track the cargo along the route

There are also a few pure domain packages that contain some intricate business-logic. They provide domain objects and services that are used by each application service to provide interesting use-cases for the user.

`inmem` contains in-memory implementations for the repositories found in the domain packages.

The `routing` package provides a _domain service_ that is used to query an external application for possible routes.

## Contributing

As with all Go kit examples you are more than welcome to contribute. If you do however, please consider contributing back to the original project as well.
