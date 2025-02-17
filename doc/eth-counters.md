# YANG to Ethtool Mapping

This column contains the mapping between YANG and Linux / Ethtool counters.

```
┌─────────────────────────────────┬──────────────────────────────────┐
│ YANG                            │ Linux / Ethtool                  │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-total-octets                 │ FramesReceivedOK,                │
│                                 │ FrameCheckSequenceErrors         │
│                                 │ FramesLostDueToIntMACRcvError    │
│                                 │ AlignmentErrors                  │
│                                 │ etherStatsOversizePkts           │
│                                 │ etherStatsJabbers                │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-frames                       │ FramesReceivedOK                 │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-multicast-frames             │ MulticastFramesReceivedOK        │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-broadcast-frames             │ BroadcastFramesReceivedOK        │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-error-fcs-frames             │ FrameCheckSequenceErrors         │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-error-undersize-frames       │ undersize_pkts                   │
├─────────────────────────────────┼──────────────────────────────────┤
| in-error-oversize-frames        | etherStatsJabbers,               |
|                                 | etherStatsOversizePkts           |
├─────────────────────────────────┼──────────────────────────────────┤
│ in-error-mac-internal-frames    │ FramesLostDueToIntMACRcvError    │
├─────────────────────────────────┼──────────────────────────────────┤
│ out-frames                      │ FramesTransmittedOK              │
├─────────────────────────────────┼──────────────────────────────────┤
│ out-multicast-frames            │ MulticastFramesXmittedOK         │
├─────────────────────────────────┼──────────────────────────────────┤
│ out-broadcast-frames            │ BroadcastFramesXmittedOK         │
├─────────────────────────────────┼──────────────────────────────────┤
│ infix-eth:out-good-octets       │ OctetsTransmittedOK              │
├─────────────────────────────────┼──────────────────────────────────┤
│ infix-eth:in-good-octets        │ OctetsReceivedOK                 │
└─────────────────────────────────┴──────────────────────────────────┘
```
