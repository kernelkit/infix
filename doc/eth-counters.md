# YANG to Ethtool Mapping
This column contains the mapping between YANG and Linux / Ethtool counters.

```
┌─────────────────────────────────┬──────────────────────────────────┐
│ YANG                            │ Linux / Ethtool                  │
├─────────────────────────────────┼──────────────────────────────────┤
│ out-frames                      │ FramesTransmittedOK              │
├─────────────────────────────────┼──────────────────────────────────┤
│ out-multicast-frames            │ MulticastFramesXmittedOK         │
├─────────────────────────────────┼──────────────────────────────────┤
│ out-broadcast-frames            │ BroadcastFramesXmittedOK         │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-total-frames                 │ FramesReceivedOK,                │
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
│ in-error-undersize-frames       │ undersize_pkts                   │
├─────────────────────────────────┼──────────────────────────────────┤
│ in-error-fcs-frames             │ FrameCheckSequenceErrors         │
└─────────────────────────────────┴──────────────────────────────────┘
```
