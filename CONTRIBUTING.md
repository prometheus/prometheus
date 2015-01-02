# Contributing

Prometheus uses GitHub to manage reviews of pull-requests.

## Getting Started

1. Reach out via our [mailing list](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers) and ask us what
   the current priorities are. We can find a good isolated starter project for you.

2. Keeping code hygiene is important.  We thusly have a practical preference
   for the following:

   1. Run ``make format`` to ensure the correctness of the Go code's layout.

   2. Run ``make advice`` to find facial errors with a static
      analyzer. In addition, consider running
      [`golint`](https://github.com/golang/lint).

   3. Try to capture your changes in some form of a test.  Go makes it easy to
      write [Table Driven Tests](https://code.google.com/p/go-wiki/wiki/TableDrivenTests).
      There is no mandate to use this said scaffolding mechanism, but it _can_
      make your life easier in the right circumstances.

   4. Relevant style guidelines are the [Go Code Review
      Comments](https://code.google.com/p/go-wiki/wiki/CodeReviewComments)
      and the _Formatting and style_ section of Peter Bourgon's [Go:
      Best Practices for Production
      Environments](http://peter.bourgon.org/go-in-production/#formatting-and-style).

3. Welcome aboard!
