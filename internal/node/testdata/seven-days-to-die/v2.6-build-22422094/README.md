# 7 Days to Die management baseline

This fixture set was captured on 2026-08-28 from a supported local 7 Days to Die dedicated server running **V2.6 Stable (b14)**, Steam build **22422094**, and native dashboard API **1.0.0**.

The baseline contains the dashboard OpenAPI master and referenced fragments, the live command catalog and detail/help responses, sanitized Player identities, user- and command-permission exchanges, representative result cases, and exhaustive command/native-API inventories. Timeout and unavailable-read-back cases are deterministic transport simulations; successful execution, rejection, and permission read-back cases were observed against the live server.

Sanitization replaces dashboard credentials, addresses, filesystem paths, timestamps, Player names, Player identifiers, location/status values, and other Player-identifying fields with deterministic neutral values. Only header names are retained. The fixture validation test rejects missing coverage, hash drift, or sensitive value patterns.

The opt-in integration capture requires `XYLONA_CAPTURE_7DTD_BASELINE=1` plus `XYLONA_7DTD_BASE_URL`, `XYLONA_7DTD_TOKEN_NAME`, and `XYLONA_7DTD_TOKEN_SECRET`. Use `XYLONA_VERIFY_7DTD_BASELINE=1` with the same connection variables to compare a future live command catalog and help data without replacing the committed baseline.

The inventories describe the tested baseline; they do not add execution behavior. Excluded operations retain Console as a manual escape hatch, not an asserted equivalent transport.
