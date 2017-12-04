# Prometheus Governance v1.0

This document describes the rules and governance of the project. It is meant to be followed by all the developers of the project and the Prometheus community. Common terminology used in this governance document are listed below:

* **Core Developers**: The stewards of the Prometheus community

* **Committers**: Developers who have write access to a project

* **Maintainers**: Committers who are leads on an individual project (MAINTAINERS.md)

* **Projects**: Any repository in the Prometheus [GitHub organization](https://github.com/prometheus)

## Values

The Prometheus developers and community are expected to follow the values defined in the [CNCF charter](https://www.cncf.io/about/charter/), including the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md). Furthermore, the Prometheus community strives for kindness, giving feedback effectively and building a welcoming environment. The Prometheus developer community strives to operate by rough consensus for the majority of decisions and only resort to conflict resolution through a motion if consensus cannot be reached.

## Core Developers

As stewards of the project, Core Developers have additional duties beyond that of a maintainer. They are expected to represent the best interests of the project, above any employer or other company/organisational ties. The current Core Developers are:

* Brian Brazil, Fabian Reinartz, Julius Volz

New Core Developers are proposed by Core Developers on the prometheus-team@ mailing list, and voted on using the rules discussed in the voting section below. The current Core Developers are recognized as leaders of the Prometheus project as a whole, through their sustained and significant contributions across the project.

## Core Developer Lead

Core Developers are allowed to elect a Core Developer Lead who serves as the final say in any deadlock situation that can’t be resolved via a vote of the Core Developers or any maintainers. Instead of calling a vote, Core Developers and maintainers may always delegate a decision to the Core Developer Lead. However, if a vote is called, it takes precedence over the decision of the Core Developer Lead.

## Projects

Each project must have a MAINTAINERS.md file with at least one maintainer. Where a project has a release process, access and documentation should be such that more than one person can perform a release. Releases should be announced on the prometheus-users mailing list. Any new repositories should be first proposed on prometheus-team@ and following the voting procedures listed below. When a repository is no longer relevant, it should be moved to the prometheus-junkyard Github organization.

## Committers and Maintainers

Committer status (commit bit) is given to those who have made ongoing contributions to a given project. This is usually in the form of code improvements, notable work on documentation, organizing events or user support could also be taken into account. 

Maintainers are committers who are expected to lead the project and serve as a point of conflict resolution amongst the committers.

In order to become a committer or maintainer, an existing maintainer must nominate you on the public prometheus-developers@ mailing list. Once another maintainer (or core developer) seconds and there are no objections within a week, a core developer will add the nominee to the prometheus-team@ mailing list and give appropriate other access rights. If there is an objection which can't be resolved via consensus, a formal [vote](#heading=h.x907eu6orfvp) will occur. A maintainer or committer may resign by notifying the prometheus-team@ mailing list. A maintainer with no project activity for a year will be deemed to have resigned.

## Voting

The Prometheus project usually runs by informal consensus, however sometimes a formal decision must be made. All votes MUST be done on the prometheus-developers@ mailing list, and generally at least a week given for discussion between maintainers and for votes to take place. A majority of Core Developers must vote in favor for a vote to pass.

Formal votes must happen for the following: major changes to this document or the project structure, financial matters, adding core developers, removing committers or core developers.

## Releases

The release process hopes to encourage early, consistent consensus-building during project development. The mechanisms used are regular community communication on the mailing list about progress, scheduled meetings for issue resolution and release triage, and regularly paced and communicated releases. Each project within Prometheus can define their own release process if there’s consensus amongst the maintainers.

## FAQ

**How do I propose a vote?**

Send an email to prometheus-dev with your motion. A majority of Core Developers must vote in favor (+1 or LGTM) for a vote to pass.

**How do I add a project or committer?**

In order to become a committer or maintainer, an existing maintainer must nominate you on the public prometheus-dev@ mailing list. Once another maintainer (or core developer) seconds and there are no objections within a week, a core developer will add the nominee to the prometheus-team@ mailing list and give appropriate other access rights. If there is an objection which can't be resolved via consensus, a formal [vote](#heading=h.x907eu6orfvp) will occur. 

**How do I remove a committer or maintainer?**

A maintainer or committer may resign by notifying the prometheus-team@ mailing list. A maintainer with no project activity for a year will be deemed to have resigned.

**How do I archive or remove a project?**

Send an email to prometheus-dev stating that a repository is no longer relevant, then it should be moved to the prometheus-junkyard Github organization.
