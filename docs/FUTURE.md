# Future work

Ideas that are not built yet, noted so they are not forgotten. None of these block the current milestones.

## Features a real auction business would want

- **Real payments.** A deposit only credits the wallet. There is no card or bank payment, no withdrawal and no refund. The ledger is the right place to plug a payment provider in behind.
- **Proxy (maximum) bidding.** The standard feature on auction sites: the bidder sets a ceiling and the system bids for them. The API has no such endpoint.
- **Reserve price enforcement.** The catalogue shows a reserve label. Check that the backend enforces a reserve before a sale completes.
- **After-sale flow.** Settlement moves the money and stops. There is no shipping, delivery confirmation or dispute step.
- **Profiles and reputation.** No profile page, seller rating or feedback.
- **Notifications outside the site.** Outbid and won alerts only appear in the browser. Email or push would reach people who are away.
- **Password reset and account deletion.**

## Hardening

- **Photo storage.** Uploaded photos stay in the uploader's browser. Real listings need object storage.
- **Admin tooling.** There is reconcile and outbox retry, but no way to ban a user, cancel a fraudulent auction or look up one user's activity.
- **Rate-limit identity.** Behind a proxy, limits depend on the trusted-proxy setting being right. The demo bots need limits off because they share one IP.
- **A very hot auction.** One row lock per auction is right at this size. Tens of thousands of simultaneous bidders on one lot would need a different design. See `docs/PERFORMANCE.md` for where the current limit is.
- **Backup and restore drills.** The scripts exist; rehearse them.
