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
  - Local `make -f sensitivityscore.mk ss-image` is still fine for the LOCAL
    dev cluster (imagePullPolicy: Always picks up a local rebuild) — just
    never push what it builds.
  - Adopting a release on the stand: set `SCHEDULER_RELEASE_VER := <tag>` in
    the sibling repo's `Makefile`, then `make scheduler-deploy` from there.
- Remember: after a redeploy on the SAME tag, a running cluster needs
  `kubectl rollout restart deployment/sensitivityscore-scheduler` —
  `kubectl set image` to the *same* tag is a no-op and won't actually
  restart the pod. (Adopting a NEW tag via `make scheduler-deploy` does not
  have this problem.)
