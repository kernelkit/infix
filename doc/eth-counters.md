# RMON Counters

The Ethernet frame counters of a port are read on request with the
`get-statistics` action on the Ethernet container of the interface,
which `show interface <port>` runs for you.  A port whose driver keeps
no IEEE 802.3 counters answers without any.

This document shows the mapping between the YANG leaves of the action
output and the Linux / Ethtool counters.

| **YANG**                       | **Linux / Ethtool**                                                                                                                               |
|--------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `in-total-frames`              | `FramesReceivedOK`, `FrameCheckSequenceErrors`, `FramesLostDueToIntMACRcvError`, `AlignmentErrors`, `etherStatsOversizePkts`, `etherStatsJabbers` |
| `in-frames`                    | `FramesReceivedOK`                                                                                                                                |
| `in-multicast-frames`          | `MulticastFramesReceivedOK`                                                                                                                       |
| `in-broadcast-frames`          | `BroadcastFramesReceivedOK`                                                                                                                       |
| `in-error-fcs-frames`          | `FrameCheckSequenceErrors`                                                                                                                        |
| `in-error-undersize-frames`    | `etherStatsUndersizePkts`                                                                                                                         |
| `in-error-oversize-frames`     | `etherStatsJabbers`, `etherStatsOversizePkts`                                                                                                     |
| `in-error-mac-internal-frames` | `FramesLostDueToIntMACRcvError`                                                                                                                   |
| `in-good-octets`               | `OctetsReceivedOK`                                                                                                                                |
| `out-frames`                   | `FramesTransmittedOK`                                                                                                                             |
| `out-multicast-frames`         | `MulticastFramesXmittedOK`                                                                                                                        |
| `out-broadcast-frames`         | `BroadcastFramesXmittedOK`                                                                                                                        |
| `out-good-octets`              | `OctetsTransmittedOK`                                                                                                                             |
