# Working agreements

- `git commit` no longer needs my approval: commit on your own once a
  logical unit of work is done and verified (build passes).
  Keep commits scoped and messages explanatory, as before.
- After any commit (yours or mine), `git push` the current branch to `origin`
  (`git@github.com:AndreyZa/scheduler-plugins.git`) automatically, without
  asking first. Unconditional — applies regardless of what the commit
  touches (e.g. a `sensitivityscore.mk`-only or doc-only commit still gets
  pushed, just with no image rebuild below).
- Rebuilding + `docker push`-ing the image is **conditional**: only do it
  when the commit actually touches `pkg/sensitivityscore/**` or other code
  that ends up in the scheduler binary — not on every commit.
  - Rebuild: from `../sensitivityscore-hpc-bench`, `make scheduler-plugin-image`
    (or `make -f sensitivityscore.mk ss-image` from here directly).
  - `docker push` it to Docker Hub (`andreyza/sensitivityscore:<tag>`, tag
    from `SCHEDULER_RELEASE_VER` in the sibling repo's `Makefile`).
- Remember: after rebuilding, a running cluster needs `kubectl rollout restart deployment/sensitivityscore-scheduler` — `kubectl set image` to the *same* tag is a no-op and won't actually restart the pod.
