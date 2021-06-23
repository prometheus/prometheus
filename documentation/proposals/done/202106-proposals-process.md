# 2021-06: Proposal Process

* **Owners:**:
  * [`@bwplotka`](https://github.com/bwplotka)

* **Other docs:**
  * [KEP Process](https://github.com/kubernetes/enhancements/blob/master/keps/README.md)
  * [RHOBS Handbook Proposal Process](https://rhobs-handbook.netlify.app/proposals/done/202106-proposals-process.md/)
  * [Thanos Proposal Process](https://deploy-preview-4366--thanos-io.netlify.app/tip/contributing/proposal-process.md/)

> TL;DR: We would like to propose an improved, official proposal process for Prometheus that clearly states when, where and how to create proposal/enhancement/design documents. We propose to store done, accepted and rejected proposals in markdown in the main Prometheus repo.

## Why

More extensive architectural, process, or feature decisions are hard to explain, understand and discuss. It takes a lot of time to describe the idea, to motivate interested parties to review it, give feedback and approve. That's why it is essential to streamline the proposal process.

Given that as open-source community we work in highly distributed teams and work with multiple communities, we need to allow asynchronous discussions. This means it's essential to structure the talks into shared documents. Persisting those decisions, once approved or rejected, is equally important, allowing us to understand previous motivations.

There is a common saying [`"I've just been around long enough to know where the bodies are buried"`](https://twitter.com/AlexJonesax/status/1400103567822835714). We want to ensure the team related knowledge is accessible to everyone, every day, no matter if the team member is new or part of the team for ten years.

### Pitfalls of the current solution

Prometheus does not have official proposal process. The unofficial process is to create a proposal either in Google Doc or directly in GH issue or in dev mailing list. This inconsistency is leading to fragmentation of reviewers and process being overall not clear. This was the direct feedback of past Prometheus mentees.

Specifically current flow has following disadventages:

* Current pending proposals are hard to discover makes it harder for maintainers to follow up on them.
* Past accepted / in limbo state / rejected proposals are impossible to track. This leads to
  * Less value in creating such formal proposal.
  * Repeated questions and researching on the idea that was previously researched.
  * Lack of awareness of past motivations for the decision.
* Not clear process makes it less likely for contributor to actually spend time on proper proposal for the desired idea.

## Goals

Goals and use cases for the solution as proposed in [How](#how):

* Allow easy collaboration and decision making on design ideas.
* Have a consistent design style that is readable and understandable.
* Ensure design docs are discoverable for better awareness and knowledge sharing about past decisions.
* Define a clear review and approval process.

## Non-Goals

* Define process for other type of documents.

## How

We want to propose an improved, official proposal process for Prometheus Community that clearly states *when, where and how* to create proposal/enhancement/design documents.

See [the full proposed process and template defined here](../proposal-process.md)

## Alternatives

1. Put proposals in a separate repository in Prometheus org

There is no harm in putting those in main repository. Spreading this info somewhere else will harm discoverability and IMO does not bring any additional value.

2. Put proposals in a docs repository in Prometheus org and allow it to be part of prometheus.io website.

In my opinion the website should be focused on users. It might be confusing and surprising for users to read past accepted / rejected proposals there.