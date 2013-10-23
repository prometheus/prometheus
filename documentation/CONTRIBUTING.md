# Contributing

Prometheus uses Gerrit to manage reviews of pull-requests, and then
Gerrit replicates its master branch to GitHub. In order to contribute to
Prometheus, you must use Gerrit.

## Setup

1. Sign in at http://review.prometheus.io/
2. Set a username and upload an SSH pubkey for git ssh access. `cat
   ~/.ssh/id_rsa.pub | pbcopy` will copy your public key to your
   clipboard so you can paste it.
3. Clone the repo: `git clone http://review.prometheus.io/prometheus`
4. Add your user-specific remote that you will push your changes to:
   `git remote add <your-remote-name> ssh://<username>@review.prometheus.io:29418/prometheus`
4. Add Change-Id commit hook: "curl -o .git/hooks/commit-msg http://review.prometheus.io/tools/hooks/commit-msg"
6. Make the file executable: `chmod u+x .git/hooks/commit-msg`
7. Commit any local changes to git, then:
8. `git push <your-remote-name> HEAD:refs/for/master`
9. Assign reviewer for change at http://review.prometheus.io/

That's all!
