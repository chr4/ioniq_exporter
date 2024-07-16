# Prometheus exporter for Hyundai (Bluelink) vehicles

This Prometheus exporter connects to Hyundai cloud services to receive vehicle metrics.

![build](https://github.com/chr4/ioniq_exporter/workflows/build/badge.svg)

It's currently retrieving the following values:

- Eletric vehicle range (`ev_range` value as integers).
- Eletric vehicle state of charge (`ev_soc` value as integers (percentages)).
- Vehicle status (`ev_status` return the charging state, see below for explanation).
- Charging finish time (`ev_finish_time` returns a UNIX timestamp, but in scientific notation.
- Odometer (`ev_odometer` returns the total distance travelled by the vehicle as integer).

It should work with all Hyundai/ KIA EVs, but is mainly tested with the Hyundai Ioniq 5.

All the heavy lifting is done by [evcc libraries](https://github.com/evcc-io/evcc/).

## Explanation of vehicle status values

Below are the values of the `ev_status`-field and how they can be interpreted.

| Value| Plugged-in | Charging | Explanation                                                                                  |
|------|------------|----------|----------------------------------------------------------------------------------------------|
|    0 |          ? |        ? | No status can be determined                                                                  |
|    1 |          N |        N | Car is not plugged-in                                                                        |
|    2 |          Y |        N | Car plugged-in but the vehicle is not charging                                               |
|    3 |          Y |        Y | Car plugged-in and is charging                                                               |
|    4 |          Y |        Y | Car plugged-in and charging, but with external ventilation request (for lead-acid batteries) |
|    5 |          Y |        N | Car plugged-in, but not charging due to error: cable error (CP short circuit, 0V)            |
|    6 |          Y |        N | Car plugged-in, but not charging. Simulate EVSE or unplugging error (CP wake-up, -12V)       |
-------------------------------------------------------------------------------------------------------------------------------
