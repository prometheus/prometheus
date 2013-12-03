# Contributing

Prometheus uses Gerrit to manage reviews of pull-requests, and then
Gerrit replicates its master branch to GitHub. In order to contribute to
Prometheus, you must use Gerrit.

## Setup

1. Sign in at http://review.prometheus.io/
2. Set a username and upload an SSH pubkey for git ssh access.

   On OSX you can use `cat ~/.ssh/id_rsa.pub | pbcopy` to copy your public key
   to your clipboard so you can paste it.
3. Clone the repo: `git clone http://review.prometheus.io/prometheus`
4. Add your user-specific remote that you will push your changes to:
   `git remote add <your-remote-name> ssh://<username>@review.prometheus.io:29418/prometheus`
5. Add Change-Id commit hook: `curl -o .git/hooks/commit-msg http://review.prometheus.io/tools/hooks/commit-msg`
6. Make the file executable: `chmod u+x .git/hooks/commit-msg`
7. Commit any local changes to git, then:
8. `git push <your-remote-name> HEAD:refs/for/master`
9. Assign reviewer for change at http://review.prometheus.io/

## Getting Started

1. Reach out via our [mailing list](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers) and ask us what
   the current priorities are.  We can find a good isolated starter project for
   you.

2. Keeping code hygiene is important.  We thusly have a practical preference
   for the following:

   1. Run ``make format`` to ensure the correctness of the Go code's layout.

   2. Run ``make advice`` to find facial errors with a static analyzer.

   3. Try to capture your changes in some form of a test.  Go makes it easy to
      write [Table Driven Tests](https://code.google.com/p/go-wiki/wiki/TableDrivenTests).
      There is no mandate to use this said scaffolding mechanism, but it _can_
      make your life easier in the right circumstances.

3. Welcome aboard!
