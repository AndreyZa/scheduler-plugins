# Working agreements

- `git commit` no longer needs my approval: commit on your own once a
  logical unit of work is done and verified (build passes).
  Keep commits scoped and messages explanatory, as before.
- After any commit (yours or mine), `git push` the current branch to `origin`
  (`git@github.com:AndreyZa/scheduler-plugins.git`) automatically, without
  asking first. Unconditional — applies regardless of what the commit
  touches (e.g. a `sensitivityscore.mk`-only or doc-only commit still gets
  pushed, just with no image rebuild below).
- **Releases go through CI only** (decided 2026-08-02). Do NOT `docker push`
  the plugin image by hand: the push to `master` already IS the release —
  the `sensitivityscore` workflow builds via the same `ss-release` target
  and publishes an immutable `andreyza/sensitivityscore:v<date>-<commit>`
  (tag printed in the run summary). A hand-pushed image would bypass tests
  and produce a tag whose provenance nobody can trace to CI.
  - There is no local dev cluster any more (kind/STAGE are gone since
    August; the lab runs k0s). `make -f sensitivityscore.mk ss-image` is
    for a local smoke build only — never push what it builds.
  - CI stays on GitHub-hosted `ubuntu-latest` on purpose: the repo is
    public (minutes are free) and workflows come from upstream. Do not move
    it to the lab runners.
  - Changes under `pkg/trimaran/**` ship too — they are in our binary.
  - Adopting a release on the stand: set `SCHEDULER_RELEASE_VER := <tag>` in
    the sibling repo's `Makefile`, then `make scheduler-deploy` from there.
- Remember: after a redeploy on the SAME tag, a running cluster needs
  `kubectl rollout restart deployment/sensitivityscore-scheduler` —
  `kubectl set image` to the *same* tag is a no-op and won't actually
  restart the pod. (Adopting a NEW tag via `make scheduler-deploy` does not
  have this problem.)

# Contracts with sensitivityscore-hpc-bench (break them and series fail)

- The preflight of `run-series.sh` waits for the log line
  `sensitivity weights loaded`; it must be printed on the FIRST load of
  the weights, not only on reload.
- Redis field names are a three-way contract (metrics-agent, this plugin,
  the bench harness); `make check-contract` in the bench checks it. Rename in all three
  or nowhere.
- A panic in an informer handler kills the WHOLE scheduler process, with
  every profile in it. No bare type assertions in `OnDelete`: handle the
  `cache.DeletedFinalStateUnknown` tombstone.
- No movable `:dev` tag for the plugin on purpose — the scheduler is the
  object being measured; provenance needs the immutable tag.

General rules for all repos (access paths, release/adopt/deploy terms,
what never to do on prod without an explicit ask): `~/phd/CLAUDE.md`.
