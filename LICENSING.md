English · [한국어](LICENSING.ko.md)

# License: pqcaton

**The source is open, and production use at scale takes a contract.**

It is the [Business Source License 1.1](LICENSE). You can read, change and run the source, but it is not an open-source license and it
is not on the OSI list. **"Source-available"** is the accurate term.

## What you can use without a contract

| Use | Contract |
|---|---|
| Evaluation, development, testing, CI | **Not needed.** No size limit either |
| Production (**5 or fewer** managed nodes) | Not needed |
| Any other production use | **Needed** |
| **Offering it to third parties as hosted or embedded** | **Needed**, regardless of the number of nodes |

A node counts what pqcaton **observes or applies changes to**: hosts, virtual machines and containers alike, summed across affiliates
under common control.

**Why 5**: enough to **try this product in a real environment**. Some things documents and demos cannot tell you. Which OpenSSL fork your
nodes run, or which provider your application registers at run time, you only learn by connecting it. We leave open the size that check
needs.

There is no exception for nonprofits or education. If the need is confirmed, we will widen it then. Widening after starting narrow is
possible; narrowing after starting wide hurts those already using it.

Contact: **kty@sntsoft.co.kr**

## Each release becomes open source four years later

**Change Date: four years from the release date · Change License: Apache-2.0**

**Each version has its own clock.** Whichever version you received, four years after the day it was published it becomes Apache-2.0.
That happens automatically and the company cannot stop it; the license terms themselves say so. **The conversion date of each
version is written in the [release notes](RELEASE_NOTES.md).**

| Version | Published | Converts to Apache-2.0 |
|---|---|---|
| v0.1.0 ~ v0.5.0 | 2026-08-18 | **2030-08-11** |
| v0.6.0 and later | each release date | four years from that day |

v0.1.0 to v0.5.0 convert seven days early (it would be 2030-08-18 by release date) because those versions were distributed under a
license that named a fixed date (2030-08-11). **A promise already made is not taken back.** For those who received those versions, that
date stays valid.

There is one reason for this term. **A source-available license is suspected of "maybe closing one day."** Naming the deadline in the
license removes that suspicion. Even if the company disappears, versions already released open on their set dates.

Four years is also the **maximum BUSL allows**. The license text sets it as *"the Change Date, or the fourth anniversary of the first
publicly available distribution of a specific version, whichever comes first"*, so there is no way to make it later.

## Why it is not open source

pqcaton is **a tool used inside an organization**. Putting a copyleft license such as AGPL on such software creates no real obligation:
used in-house without distribution, there is nothing to publish. If the license cannot tell free use from paid use, the people making it
cannot keep making it.

BUSL **governs use, not distribution.** That is why it fits this product.

## The source is open anyway

pqcaton observes an organization's cryptographic assets and handles migration decisions. **Putting a tool there whose judgments you
cannot read is dangerous**, all the more for an organization that is audited.

So the code is open. You can read it, verify it and change it to fit your environment. A contract is needed only to use it in production
at scale.

## The base project: pqcota

pqcaton uses the public modules of [pqcota](https://github.com/randyinthedev-hash/pqcota) (Apache-2.0) (`pqcota-common`,
`pqcota-inventory`, `pqcota-discovery`, `pqcota-provisioning`) as Go packages. pqcota **observes and generates changes**, and pqcaton
**reconciles, reviews and finalizes** on top of it. The two have different owners, and the dependency goes one way: pqcaton uses pqcota,
never the reverse.

Apache-2.0 allows commercial downstream use, so this structure has no constraint. The attribution notice is in [NOTICE](NOTICE).
**pqcota is complete open source on its own and can be used without pqcaton.**

## Contributions and the license

Converting to open source on the Change Date requires **the right to release all of the code that way**. So outside contributions go
through the [CLA](CONTRIBUTING.md#contributor-license-agreement-cla).

> **This is not legal advice.** This document is a guide; where it differs, the [LICENSE](LICENSE) and the actual contract prevail.
