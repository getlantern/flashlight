# Flashlight tester

This is a simple tester for the Flashlight library.
It receives all it's arguments via environment variables.

`FETCH_TIMEOUT_SECONDS` defaults to 20 and accepts 1–600. A process deadline also
bounds connection startup. Only HTTPS URLs and redirects are accepted.

For normal configuration fetching, `ENVIRONMENT` and `CONFIG_URL` must explicitly
match: `staging` uses `https://api.staging.iantem.io/v1/config`; `prod` uses
`https://api.iantem.io/v1/config`. Sticky tests use the supplied `user.conf`.

`fetch.json` contains curl's HTTP status, exit code, total and first-byte timings;
the response body is in `output.txt`. An HTTP error from the origin still proves
reachability. `connection.txt` is `connecting` until the tunnel connects, then
`connected`. A stopped tester with `connecting` and no fetch metadata failed to
establish the tunnel. Missing both files indicates a tester or configuration
failure. Old result artifacts are cleared before each run.

## Environment variables

- `DEVICE_ID`: The device id to use.
- `USER_ID`: The user id to use.
- `TOKEN`: The token to use.
- `RUN_ID`: The run id to use. It will be added to honeycomb traces as value of attribute `pinger-id`. (you can use it for looking up traces for the specific run)
- `TARGET_URL`: The target url to use. This is the url that will be pinged through flashlight.
- `DATA`: The path to use for config files and for logs. This is the path where the output will be written to (proxies.yaml, global.yaml, etc). You can place proxies.yaml there to use it instead of fetching.

All of these are required.

## CLI usage

```bash
ENVIRONMENT=staging CONFIG_URL=https://api.staging.iantem.io/v1/config \
SIGNOZ_INGESTION_KEY=your-ingestion-key \
DEVICE_ID=1234 USER_ID=123 TOKEN=1234 RUN_ID=1234 TARGET_URL=https://example.com DATA=./mydir ./flashlight-tester
```

The tester will start flashlight, fetch the config & proxies and try to reach the target URL via the socks5 proxy that flashlight provides.
Upoon success, it will write the output of that request to the `output.txt`.

## Docker usage

On each new push to the repository, a new image of the tester is built and pushed to the registry.
It's tagged as `us-docker.pkg.dev/lantern-cloud/containers/flashlight-tester:FLASHLIGHT_HASH`


```bash
docker run --rm -v ./mydir:/output \
    -e ENVIRONMENT=staging \
    -e CONFIG_URL=https://api.staging.iantem.io/v1/config \
    -e SIGNOZ_INGESTION_KEY \
    -e DEVICE_ID=1234 \
    -e USER_ID=1234 \
    -e TOKEN=1234 \
    -e RUN_ID=1234 \
    -e TARGET_URL=https://example.com \
    -e DATA=/output \
   us-docker.pkg.dev/lantern-cloud/containers/flashlight-tester
```

## Passing custom proxies.yaml

If you want to use a custom proxies.yaml, you can place it in the output directory and it will be used instead of fetching it from the server.
In order for flashlight to pick it up instead of using the fetched config, you need to specify another env variable: `STICKY=true`
