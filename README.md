MaxiFeed
========

An RSS reader you actually keep up with.

MaxiFeed is a personal fork of [Miniflux](https://miniflux.app). Miniflux's unread list suits a desktop, but on a phone it's easy to forget until 200 posts are waiting.
MaxiFeed turns that list into a news feed: you scroll, and it clears itself.

- **Scroll to clear:** posts are marked as read as they pass.
- **Swipe to decide:** right to save, left to skip.
- **Batches, not backlogs:** 25 posts at a time, with a finish line.
- **Fair feeds:** feeds take turns, so a busy one can't bury a quiet one.
- **Long reads in stints:** saved articles resume at your paragraph on any device, show the time left, and stay up top until you finish them.
- **Readeck and Raindrop:** send a post to either in two taps, and see whether it arrived.
- **Daily reminder:** a notification at the time you pick, with no third-party account.
- **Streaks:** one more reason to open it tomorrow.
- **Installable:** add it to your home screen and it opens like an app.

Everything else in [Miniflux](https://miniflux.app/features.html) still works, including integrations, OPML import and export, the API, and the classic list views under More.

Running it
----------

MaxiFeed is one Go binary plus PostgreSQL, configured with the same [environment variables as Miniflux](https://miniflux.app/docs/configuration.html). Existing Miniflux databases upgrade in place when `RUN_MIGRATIONS=1` is set.

Build a Linux binary with Go 1.26, for `amd64` or `arm64`:

```bash
scripts/build-binary.sh amd64
```

Or replace Miniflux in a systemd service over ssh, with an automatic rollback if the new version won't start:

```bash
scripts/deploy.sh your-server [service]
```

Notifications need HTTPS, and on iPhone the app must be installed first. Integrations on a private address need `INTEGRATION_ALLOW_PRIVATE_NETWORKS=1`.

License
-------

Apache 2.0. Built on [Miniflux](https://github.com/miniflux/v2) by Frédéric Guillot and contributors.
