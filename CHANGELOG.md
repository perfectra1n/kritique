# Changelog

## 0.1.0 (2026-09-30)


### ⚠ BREAKING CHANGES

* kritique is not a drop-in replacement for a kritik deployment. Config keys, env vars, database roles and the repository config filename have all changed.
* **gateway:** the worker is the model gateway; no provider key enters a runner ([#27](https://github.com/perfectra1n/kritique/issues/27))
* **egress:** runner pods leave the cluster only through the worker gateway ([#24](https://github.com/perfectra1n/kritique/issues/24))
* **store:** flatten the migrations into one schema ([#22](https://github.com/perfectra1n/kritique/issues/22))
* **store:** index embeddings with VectorChord ([#21](https://github.com/perfectra1n/kritique/issues/21))
* **review:** render comments with text/template and sprout ([#17](https://github.com/perfectra1n/kritique/issues/17))
* forgejo forge, agentic reviews, in-repo config and templated output ([#15](https://github.com/perfectra1n/kritique/issues/15))

### Features

* **agent:** run allowlisted commands over a checkout of the head ([#26](https://github.com/perfectra1n/kritique/issues/26)) ([e5d3935](https://github.com/perfectra1n/kritique/commit/e5d3935b4d408459f381f14f3d35d176005f0220))
* **config:** leave out a file tenant whose names a dashboard tenant holds ([#55](https://github.com/perfectra1n/kritique/issues/55)) ([00651d3](https://github.com/perfectra1n/kritique/commit/00651d326cab8bb206654bcb3ace5877cfea088d))
* **config:** let .kritik.yaml choose within the operator's allow bounds ([#58](https://github.com/perfectra1n/kritique/issues/58)) ([eb01520](https://github.com/perfectra1n/kritique/commit/eb0152080d62e1121c0442bf31fd13bd6cc25cc2))
* **config:** let every scope set every repository setting, presence winning ([#53](https://github.com/perfectra1n/kritique/issues/53)) ([b73ddd7](https://github.com/perfectra1n/kritique/commit/b73ddd7e6655f58e74534f717da0be134be446af))
* **config:** move poll, onboarding and runner deadline tuning to the file ([#51](https://github.com/perfectra1n/kritique/issues/51)) ([a655067](https://github.com/perfectra1n/kritique/commit/a655067bdf6c26a9088fc4af5572ac330e7e39ce))
* **config:** one policy table for who may write each setting ([#64](https://github.com/perfectra1n/kritique/issues/64)) ([9f25274](https://github.com/perfectra1n/kritique/commit/9f25274f4f9bf8bf30ff3bbab56bbbe0e69028d3))
* **config:** tasks in the operator's file and .kritik.yaml ([1f4ad2f](https://github.com/perfectra1n/kritique/commit/1f4ad2f7b8077cabcc65d0331ec0d32ff11fe83f))
* **egress:** runner pods leave the cluster only through the worker gateway ([#24](https://github.com/perfectra1n/kritique/issues/24)) ([416e778](https://github.com/perfectra1n/kritique/commit/416e778cac256893c17ffb85a67fbfe17cb2ec93))
* **executor:** mount the agent's tools from image volumes ([#54](https://github.com/perfectra1n/kritique/issues/54)) ([70d6b10](https://github.com/perfectra1n/kritique/commit/70d6b10cefa55a93b4d162c2b1c7329422c45ee7))
* **forge:** add issue and pull request triage operations ([59ecfd1](https://github.com/perfectra1n/kritique/commit/59ecfd1ea4923dfcd1b9614bbec0e9c076dba62c))
* forgejo forge, agentic reviews, in-repo config and templated output ([#15](https://github.com/perfectra1n/kritique/issues/15)) ([6f34e8e](https://github.com/perfectra1n/kritique/commit/6f34e8ed8f9bda52f8aefcb995de3ad6079baecf))
* **forge:** support Gitea installations ([937dc64](https://github.com/perfectra1n/kritique/commit/937dc644a86afc6790efdc0b0f4a8323b556da6e))
* **gateway:** the worker is the model gateway; no provider key enters a runner ([#27](https://github.com/perfectra1n/kritique/issues/27)) ([38eaaa8](https://github.com/perfectra1n/kritique/commit/38eaaa80d0b13e604a6e1c505fadfc4ded8078b3))
* **go:** update module github.com/odvcencio/gotreesitter (v0.54.0 → v0.55.0) ([#11](https://github.com/perfectra1n/kritique/issues/11)) ([4b797a8](https://github.com/perfectra1n/kritique/commit/4b797a8cb09e534078ed302b183b78dea6546445))
* **go:** update module github.com/openai/openai-go (v1.12.0 → v3.66.0) ([#5](https://github.com/perfectra1n/kritique/issues/5)) ([cefd5b2](https://github.com/perfectra1n/kritique/commit/cefd5b2d36bab6040cf03eff96e0151ae05a9e04))
* **go:** update module golang.org/x/oauth2 (v0.36.0 → v0.37.0) ([#35](https://github.com/perfectra1n/kritique/issues/35)) ([4cc35e2](https://github.com/perfectra1n/kritique/commit/4cc35e2fed6af297bd89f9243040b567f63c29ef))
* **ingest:** offer deliveries to tasks ([2af9b74](https://github.com/perfectra1n/kritique/commit/2af9b741c6752e77769065f3eccc2302e0b04c48))
* initial import of the kritik review service ([e27f048](https://github.com/perfectra1n/kritique/commit/e27f048e560ea3d8235ae816ed350fcb9f87b964))
* **npm:** update dependency oxfmt (0.69.0 → 0.70.0) ([#4](https://github.com/perfectra1n/kritique/issues/4)) ([f311a0f](https://github.com/perfectra1n/kritique/commit/f311a0f7851f574c163a333e8512b4884310ff5c))
* **review:** add a severity floor, summary-only output and pr.event ([#59](https://github.com/perfectra1n/kritique/issues/59)) ([064a852](https://github.com/perfectra1n/kritique/commit/064a8529041c7a24602fad712951f3e501172723))
* **review:** name reference files for the reviewer in review.context ([#61](https://github.com/perfectra1n/kritique/issues/61)) ([e66f42e](https://github.com/perfectra1n/kritique/commit/e66f42e917cd108ec0dde218aecb5da7aef2e98c))
* **review:** offer a finding's fix as a suggestion and an agent prompt ([#19](https://github.com/perfectra1n/kritique/issues/19)) ([380bbe0](https://github.com/perfectra1n/kritique/commit/380bbe0bd06d4fff0b8335b0fd75c69245d83a3d))
* **review:** render comments with text/template and sprout ([#17](https://github.com/perfectra1n/kritique/issues/17)) ([15525ad](https://github.com/perfectra1n/kritique/commit/15525ad5a60957a05e92ff0005c92d4f03e8a7a1))
* **review:** scope .kritik.yaml instructions to the paths they cover ([#60](https://github.com/perfectra1n/kritique/issues/60)) ([5690766](https://github.com/perfectra1n/kritique/commit/5690766bfdd39becafebf18f3e7ed53995cb1bd7))
* **review:** sharpen what the reviewer reports ([#18](https://github.com/perfectra1n/kritique/issues/18)) ([b4d5520](https://github.com/perfectra1n/kritique/commit/b4d55207ce93f81a5878226bbc261308a184e8d7))
* **runner:** run runner Jobs under a RuntimeClass ([#25](https://github.com/perfectra1n/kritique/issues/25)) ([8674b2a](https://github.com/perfectra1n/kritique/commit/8674b2ae64b72e4175e0971af870169f0f4a7a39))
* **store:** index embeddings with VectorChord ([#21](https://github.com/perfectra1n/kritique/issues/21)) ([23de86e](https://github.com/perfectra1n/kritique/commit/23de86e1d24721fe676fcfa2941dff3aec589069))
* **store:** read task runs and publish their status changes ([a7e214e](https://github.com/perfectra1n/kritique/commit/a7e214eec6efaf235605ce0caa551ca03b278066))
* **store:** record task events and task runs ([d89a1b5](https://github.com/perfectra1n/kritique/commit/d89a1b5b1e27622af05e44ca37c12644e9780465))
* **tasks:** adapt events to tasks and apply planned writes ([a059aaa](https://github.com/perfectra1n/kritique/commit/a059aaaf3fd734a9f9f859136669009099a9fb98))
* **tasks:** define tasks with triggers, fields, templates and actions ([ae9f8e7](https://github.com/perfectra1n/kritique/commit/ae9f8e7423781f7ccb566a9707dd4f7d06cc4205))
* **tasks:** gather context sources and run tasks agentically ([63231f8](https://github.com/perfectra1n/kritique/commit/63231f8ebfb8b7d8700a726a3e073feecdc5bcf8))
* **tasks:** run single-mode tasks and apply validated actions ([a4f41c4](https://github.com/perfectra1n/kritique/commit/a4f41c462f55615c132ed2cc8bc7e70bbd6001bb))
* web dashboard with sign-in, transcripts and dashboard-managed config ([#30](https://github.com/perfectra1n/kritique/issues/30)) ([9b8d02b](https://github.com/perfectra1n/kritique/commit/9b8d02bb6810e0a4d72927b8a60c84e22b950554))
* **webapi:** serve task runs, their records and transcripts ([f3b0116](https://github.com/perfectra1n/kritique/commit/f3b0116c873c9fd7e571d9257fe17540bf7887f1))
* **web:** ask which installation when a repository name is ambiguous ([#46](https://github.com/perfectra1n/kritique/issues/46)) ([03c89ed](https://github.com/perfectra1n/kritique/commit/03c89ed1f898ffd888b3b2ec2430f4ae24ec2678))
* **webhook:** parse issue events and keep raw deliveries ([cf3e310](https://github.com/perfectra1n/kritique/commit/cf3e310123b342e01cf03fe9baee21b289a7cdd5))
* **web:** list instance settings and their sources for operators ([#66](https://github.com/perfectra1n/kritique/issues/66)) ([ffc9185](https://github.com/perfectra1n/kritique/commit/ffc9185ac1c88d076a108ddee1be9bb269cb180e))
* **web:** list task runs and show what each applied and dropped ([d3a020b](https://github.com/perfectra1n/kritique/commit/d3a020b35bc9ba622466c1e685fc3e4fe981409a))
* **web:** move section navigation to a left sidebar ([#49](https://github.com/perfectra1n/kritique/issues/49)) ([ed71c90](https://github.com/perfectra1n/kritique/commit/ed71c9075eb01e2e51c8934e7d86e8d684253ee5))
* **web:** show each repository's resolved task definitions ([42478d2](https://github.com/perfectra1n/kritique/commit/42478d2bdd8895c33db95c0f196f92a6693a1f0c))
* **web:** show every tenant's statistics on the home page ([#50](https://github.com/perfectra1n/kritique/issues/50)) ([ba83d63](https://github.com/perfectra1n/kritique/commit/ba83d63654c8739f857ed6e75bd9c76beb4bd6ce))
* **web:** show what an empty config field inherits, and from where ([#71](https://github.com/perfectra1n/kritique/issues/71)) ([b28d0ff](https://github.com/perfectra1n/kritique/commit/b28d0ff8d1a3956ce685ddc7bde9cb0023199ef9))
* **web:** show where each repository setting comes from ([#65](https://github.com/perfectra1n/kritique/issues/65)) ([1cda26b](https://github.com/perfectra1n/kritique/commit/1cda26b37c8ce329a21948823684cb911f4c4de0))
* **worker:** read .kritik.yaml before the runner, and settle there ([#56](https://github.com/perfectra1n/kritique/issues/56)) ([dba0ecb](https://github.com/perfectra1n/kritique/commit/dba0ecb344f5e33b2c7d42cc40ad8aea02769f9b))


### Bug Fixes

* **chart:** wait for a startup probe before liveness and readiness ([#69](https://github.com/perfectra1n/kritique/issues/69)) ([d34cb40](https://github.com/perfectra1n/kritique/commit/d34cb40a02fa9dc860b9b135a8127d51dcec8cfa))
* **configfile:** key repository entries by installation and name ([#45](https://github.com/perfectra1n/kritique/issues/45)) ([5be2703](https://github.com/perfectra1n/kritique/commit/5be270375f7476cb73fcac1db8648860c3760310))
* **config:** refuse gitlab installations until there is a client ([#68](https://github.com/perfectra1n/kritique/issues/68)) ([c57671d](https://github.com/perfectra1n/kritique/commit/c57671dbe81296a3231b42cbb70312adfa29e5ce))
* **forge:** answer a mention in a Forgejo code conversation ([#75](https://github.com/perfectra1n/kritique/issues/75)) ([930cbe4](https://github.com/perfectra1n/kritique/commit/930cbe44f194a87e3a5cff7c49a92f61cd332dd6))
* **forge:** count a GitHub pull request with a deleted fork as a fork ([#73](https://github.com/perfectra1n/kritique/issues/73)) ([80fd902](https://github.com/perfectra1n/kritique/commit/80fd9022844d61aeafd3550b1e828569678440f6))
* **forge:** cut commit status descriptions by character, not byte ([#67](https://github.com/perfectra1n/kritique/issues/67)) ([8e5cb9a](https://github.com/perfectra1n/kritique/commit/8e5cb9a1b5d063f3524567a1cf879237ef4f7ac6))
* **forgejo:** add labels by id, which older Gitea releases need ([3e349a2](https://github.com/perfectra1n/kritique/commit/3e349a216015ddf56ba633fc5bb4c85cec634086))
* **forge:** resolve label name to numeric id before Forgejo DELETE ([996ca67](https://github.com/perfectra1n/kritique/commit/996ca671c8439bd52c04bf5b4a5636542f3cae5a))
* **forge:** return the oldest matching comment on Forgejo, as on GitHub ([#74](https://github.com/perfectra1n/kritique/issues/74)) ([5030b29](https://github.com/perfectra1n/kritique/commit/5030b297172e609c3b95bfa422b14acc83e173da))
* **gateway:** reserve each step against the run's budget ([#29](https://github.com/perfectra1n/kritique/issues/29)) ([686aeb0](https://github.com/perfectra1n/kritique/commit/686aeb05aa14882a17cfd18856170431514c1c90))
* **go:** update module github.com/go-git/go-billy/v5 (v5.9.0 → v5.9.1) ([#2](https://github.com/perfectra1n/kritique/issues/2)) ([1f9a6b8](https://github.com/perfectra1n/kritique/commit/1f9a6b8c80f8994cf90f6a1980240fec3a47fa91))
* **go:** update module github.com/go-jose/go-jose/v4 (v4.1.4 → v4.1.5) ([#34](https://github.com/perfectra1n/kritique/issues/34)) ([054d550](https://github.com/perfectra1n/kritique/commit/054d55041b49364043abaff74e0fb94220e36273))
* **go:** update module github.com/odvcencio/gotreesitter (v0.55.0 → v0.55.1) ([#63](https://github.com/perfectra1n/kritique/issues/63)) ([8625892](https://github.com/perfectra1n/kritique/commit/862589257309b802ad55b0b1b6f6de0275eeac03))
* **ingest:** log an undeclared account's ignored deliveries at debug ([ad87668](https://github.com/perfectra1n/kritique/commit/ad87668841ad0b419424cc918ecd3bd923fc1aab))
* keep the git token out of the Job spec, drop dead leader sessions, cap under the lease ([#13](https://github.com/perfectra1n/kritique/issues/13)) ([9f3ee1c](https://github.com/perfectra1n/kritique/commit/9f3ee1cd1ba56009a624472fbdf669af1377daac))
* **mise:** restore the lockfile a local mise lock rewrote ([c88701d](https://github.com/perfectra1n/kritique/commit/c88701d311cefdb3f8bd0ba991037ea76807334f))
* **poller:** record a first poll's older pull requests as a baseline ([#38](https://github.com/perfectra1n/kritique/issues/38)) ([196bbb0](https://github.com/perfectra1n/kritique/commit/196bbb024344c0eb3dddcf71c62e57a93cfca66f))
* **review:** no doubled blank line without findings, no "cannot verify" in the take ([#20](https://github.com/perfectra1n/kritique/issues/20)) ([9cc8709](https://github.com/perfectra1n/kritique/commit/9cc870929cc9bfcf967a430e5a79047132cb0a28))
* **store:** expire the indexes of repositories disabled past their grace ([#43](https://github.com/perfectra1n/kritique/issues/43)) ([1c0ce18](https://github.com/perfectra1n/kritique/commit/1c0ce18a7626562ead43162fc2ec8f2ecefb8c1a))
* **tasks:** accept only * and ** in allow.tasks.events ([2565628](https://github.com/perfectra1n/kritique/commit/2565628c0bad94b2c855abcbb225b82be0f1178d))
* **tasks:** bound dashboard tasks and context commands, fence template data ([929434d](https://github.com/perfectra1n/kritique/commit/929434d2d322c09b0eb72fc52288c18347fddbda))
* **tasks:** carry a pull request's draft flag into subject.draft ([6549a0a](https://github.com/perfectra1n/kritique/commit/6549a0a898cdcfcc468842871b9a488cb0391de9))
* **tasks:** fail an agentic run cut off while its agent ran instead of starting another runner ([cb87284](https://github.com/perfectra1n/kritique/commit/cb872841d35bb723457e45cf864e9c1bf593e11b))
* **tasks:** hand an agentic task's runner a git credential that can only read ([c00d7cc](https://github.com/perfectra1n/kritique/commit/c00d7cc37225f4d2c32c98c34239f1c4d462bc7b))
* **tasks:** keep a related search inside its repository and let a failed one pass ([5195c4b](https://github.com/perfectra1n/kritique/commit/5195c4bd77488a460c32f5fce8ad05aa50c5500f))
* **tasks:** never re-run an answered task run, and end every run ([65b954a](https://github.com/perfectra1n/kritique/commit/65b954a97526b25319312da469ef30588bd1db7a))
* **tasks:** render a declared context source that gathered nothing as empty ([e0d8b7a](https://github.com/perfectra1n/kritique/commit/e0d8b7a7116eb7a84f6b7f8c74e84f33d0a15e92))
* **tasks:** size agentic task jobs to their tasks, snooze for a slot, keep runner notes ([3e4a7db](https://github.com/perfectra1n/kritique/commit/3e4a7db9fa2aba730eafdcd1611187bd4d2b9024))
* **tasks:** take an agentic task's model slot before it spends anything ([3c744ed](https://github.com/perfectra1n/kritique/commit/3c744edad6d08eff65940e8b9965e7e986148ecb))
* **webapi:** count an agentic task's model calls under the task role and its repository ([eb8bda0](https://github.com/perfectra1n/kritique/commit/eb8bda0ff08fe0951499e2636429559ea3f88ca5))
* **webapi:** list a repository's tasks from the default branch tip a dispatch read ([9f53cc3](https://github.com/perfectra1n/kritique/commit/9f53cc32d5bd185f736a21bfa8b25bcba8d89c6e))
* **webapi:** show why the default branch's .kritik.yaml was ignored on the Tasks view ([c837cb0](https://github.com/perfectra1n/kritique/commit/c837cb07366197c67e77503bf8dd78b9b74cf405))
* **webhook:** normalize Forgejo label_updated to labeled ([e59afbc](https://github.com/perfectra1n/kritique/commit/e59afbc4c13bad50a6468ae7c6aa3dce5bd242ad))
* **worker:** bound jobs above their runner deadline and delete orphaned Jobs ([#12](https://github.com/perfectra1n/kritique/issues/12)) ([6a40635](https://github.com/perfectra1n/kritique/commit/6a406358d353d5604971506c0fbfe11e049c52f7))
* **worker:** clear staged chunks when an index job fails ([#42](https://github.com/perfectra1n/kritique/issues/42)) ([ab40ef8](https://github.com/perfectra1n/kritique/commit/ab40ef8abf9442ce37a1ddcd911ed9026721606d))
* **worker:** keep no transaction open while an index embeds ([#77](https://github.com/perfectra1n/kritique/issues/77)) ([c665fe2](https://github.com/perfectra1n/kritique/commit/c665fe2b21795ba14370a53de32421716c89a092))
* **worker:** pace the index queue ([#37](https://github.com/perfectra1n/kritique/issues/37)) ([bb43a03](https://github.com/perfectra1n/kritique/commit/bb43a03c5e54cfb644e96fbd5f88047a588a00b0))
* **worker:** snooze a review while every model slot is held ([#33](https://github.com/perfectra1n/kritique/issues/33)) ([3fe2df7](https://github.com/perfectra1n/kritique/commit/3fe2df714d64505b3743dc3b5cdd4923b8799ad7))
* **worker:** sweep only index runs River has given up on ([#78](https://github.com/perfectra1n/kritique/issues/78)) ([f874532](https://github.com/perfectra1n/kritique/commit/f8745329cec368a4153c91674015937692774f89))


### Performance Improvements

* **forge:** walk a Forgejo pull request's reviews once per mention ([#76](https://github.com/perfectra1n/kritique/issues/76)) ([b4b83cc](https://github.com/perfectra1n/kritique/commit/b4b83cc79e069a9d7c497477a66ec177fa6c2c9f))
* **worker:** skip an unchanged bot rebase before starting its runner ([#32](https://github.com/perfectra1n/kritique/issues/32)) ([a3f5569](https://github.com/perfectra1n/kritique/commit/a3f55695c8617acca68be09b9b5a886ddb45f417))


### Code Refactoring

* deduplicate helpers and remove quadratic loops ([#41](https://github.com/perfectra1n/kritique/issues/41)) ([af1ca71](https://github.com/perfectra1n/kritique/commit/af1ca7170e4847b68296274b314e6bb2c0909fb0))
* modernise for Go 1.27, share the worker plumbing, widen unit tests ([#10](https://github.com/perfectra1n/kritique/issues/10)) ([0374c74](https://github.com/perfectra1n/kritique/commit/0374c740004bf85db7c443025107bb62eb1f5fc9))
* rename kritik to kritique ([b56cc6f](https://github.com/perfectra1n/kritique/commit/b56cc6fef97600f289c56176df18a702b340cc2c))
* **store:** flatten the migrations into one schema ([#22](https://github.com/perfectra1n/kritique/issues/22)) ([f0ce264](https://github.com/perfectra1n/kritique/commit/f0ce2640c7ec8d1e36a8b6861f8f288964539b6d))
* **tasks:** share the task_runs.applied record type between worker and web API ([083fba5](https://github.com/perfectra1n/kritique/commit/083fba5276d6b7e73dc2527d4b3b06fdc051ab3a))


### Documentation

* **adr:** accept ADR-0010 and ADR-0011 ([#70](https://github.com/perfectra1n/kritique/issues/70)) ([68a13cd](https://github.com/perfectra1n/kritique/commit/68a13cdf6064d9b2e46d4e2353cc68a348d93fcb))
* **adr:** add ADR-0010 on configuration layers and precedence ([#44](https://github.com/perfectra1n/kritique/issues/44)) ([e0e038f](https://github.com/perfectra1n/kritique/commit/e0e038fcf82cb0b357365056db488ea64782af73))
* **adr:** add Greptile-informed .kritik.yaml content to ADR-0010 ([#48](https://github.com/perfectra1n/kritique/issues/48)) ([a95bdfa](https://github.com/perfectra1n/kritique/commit/a95bdfaa13e9b6859c47c2570599d170fdb8dbbc))
* **adr:** ADR-0003, the review is an agent in the runner, the worker its model gateway ([#14](https://github.com/perfectra1n/kritique/issues/14)) ([c30b386](https://github.com/perfectra1n/kritique/commit/c30b386ad3d1e3bba31e82481f9a4ae9c3f1b82f))
* **adr:** ADR-0008, allowlisted commands in the agent and egress through the gateway ([#23](https://github.com/perfectra1n/kritique/issues/23)) ([501b449](https://github.com/perfectra1n/kritique/commit/501b4498d3a684c68f2c93867f4095fd96ec2c39))
* **adr:** renumber the gateway ADR as 0004 amending the agentic mode ([#16](https://github.com/perfectra1n/kritique/issues/16)) ([ad9f727](https://github.com/perfectra1n/kritique/commit/ad9f72755ff2af2a09f027bc6e639b2fc067ac2f))
* **chart:** state the bound on concurrent runner pods ([#39](https://github.com/perfectra1n/kritique/issues/39)) ([03c8f65](https://github.com/perfectra1n/kritique/commit/03c8f65877de35fb7296bb12d57bd7734daf7ef0))
* publish a JSON Schema for .kritik.yaml ([#62](https://github.com/perfectra1n/kritique/issues/62)) ([1c308b5](https://github.com/perfectra1n/kritique/commit/1c308b5eac0d176891b1b15d80ca630b3082e205))
* slim the README and flag kritik as not production ready ([#40](https://github.com/perfectra1n/kritique/issues/40)) ([9d9581b](https://github.com/perfectra1n/kritique/commit/9d9581bf640a3ee2b07deeed318d0286af3c5827))
* **tasks:** add ADR-0012, the tasks reference and tested recipes ([1d748bd](https://github.com/perfectra1n/kritique/commit/1d748bd2730af0b2c954afb69d43b3ff3c67f0b0))
* **tasks:** describe where dispatch, the loop guard, retries and snoozes happen ([6a7d43a](https://github.com/perfectra1n/kritique/commit/6a7d43afc7769518616eb48858e1bda7383a1823))
* **tasks:** name the context keys and fix the tasks panel's empty state ([ca32481](https://github.com/perfectra1n/kritique/commit/ca324814ac60d9dec42c291ac49b96e6b81aecd8))
* **tasks:** note the event glob wildcards and a raw issues trigger's missing subject ([454774a](https://github.com/perfectra1n/kritique/commit/454774ad8511dd0438a3881390908ea0a5d4d7b7))
* **tasks:** say where an agentic task's glob files and commands land ([a3cedc2](https://github.com/perfectra1n/kritique/commit/a3cedc271b3206f78691c0074009d1b8d1abb0dc))


### Tests

* **gateway:** count only the suite's own gateway tokens ([#28](https://github.com/perfectra1n/kritique/issues/28)) ([2b7cc81](https://github.com/perfectra1n/kritique/commit/2b7cc8177a9d207329a14b80c88632649ca9f343))


### Build System

* **chart:** keep the generated README and schema out of the formatter ([#9](https://github.com/perfectra1n/kritique/issues/9)) ([1e3fd18](https://github.com/perfectra1n/kritique/commit/1e3fd187d4b4ff0443668ee0ce295c79f966d14f))
* **mise:** add shellcheck ([c99874c](https://github.com/perfectra1n/kritique/commit/c99874c2df19e8f4b1957121f8738c620d067019))
* **mise:** lock node without a machine-local compile option ([#31](https://github.com/perfectra1n/kritique/issues/31)) ([8082243](https://github.com/perfectra1n/kritique/commit/8082243c20c36b242f694f64578bab88e6c80f99))
* **mise:** stop tracking the lock sidecars ([#8](https://github.com/perfectra1n/kritique/issues/8)) ([b695f42](https://github.com/perfectra1n/kritique/commit/b695f4264afaedfd1e39e3c1a7709232362add5b))


### Continuous Integration

* **release:** start the version series at 0.1.0 ([#7](https://github.com/perfectra1n/kritique/issues/7)) ([120b478](https://github.com/perfectra1n/kritique/commit/120b4782a46ecc81ac28a567a7d5e0c55a38f169))
* **renovate:** install the chart tools and allow the shared presets' commands ([5eff3e9](https://github.com/perfectra1n/kritique/commit/5eff3e9280191ceb4a2d3103747a08d357cb2bf1))
* **renovate:** let Renovate set commit statuses ([cacb142](https://github.com/perfectra1n/kritique/commit/cacb1426070dd039a5259f55809315f2a566c6eb))
* **renovate:** set platformCommit to enabled, its current value ([1bb5af4](https://github.com/perfectra1n/kritique/commit/1bb5af4c8ae8ce57c25036cf6b5ff7d7606987d6))
* run Renovate, release-please and stale with the built-in token ([9e9ac24](https://github.com/perfectra1n/kritique/commit/9e9ac24848bd2d9d6c12b6846214e4f86e30906a))


### Miscellaneous Chores

* **mise:** update tool oxfmt (0.69.0 → 0.70.0) ([#3](https://github.com/perfectra1n/kritique/issues/3)) ([ec201c5](https://github.com/perfectra1n/kritique/commit/ec201c50f5dac14efba42d6526a6e776dcec637d))
* **store:** drop the config projections nothing reads ([#52](https://github.com/perfectra1n/kritique/issues/52)) ([51bcb46](https://github.com/perfectra1n/kritique/commit/51bcb46926811a0276c0bba51ccc71bd52fe9d75))
