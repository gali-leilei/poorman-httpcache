
# Overview

all command has http request/response cache via redis.

> planned:

- `cachev0` (deployed to `cachev0`): proxy only. Use original service key. Metric unlogged.
- `cachev1` (deployed to `cachev1`): proxy only. Use a single private key. Metric unlogged.
- `admin` (not deployed): add user and key in postgres. for `cachev2` and `cachev3` only.
- `staff` (deployed to `staff`):输入电邮，会拿到 proxy key. for `cachev2` and `cachev3` only. check spam folder.

> planned:

- `cachev2` (coming, will deploy to `cachev2`): proxy + postgres. Pne key per person. Metric logged in postgres. High QPS.
- `cachev3` (coming, will deploy to `cachev3`): proxy + postgres + redis. One key per person. Metric logged in redis, then sync to pg. Higher QPS than `api1` (supposedly?).
- `community` (coming, will deploy to `api-gateway`): same as `cachev3`, but with rate limiting + multi-node deployment to handle high traffic.
