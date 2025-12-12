# System Tuning Guide

## Memory

Default memory tuning is defined in `/etc/sysctl.d/vm.conf`, optimized for embedded network devices with 512MB-4GB RAM.

### For Systems with 4GB+ Memory

Systems with more memory can afford to be less aggressive with cache reclaim.

```conf
# Allow more dirty pages before writeback
vm.dirty_ratio=15
vm.dirty_background_ratio=10

# Less aggressive watermark (memory pressure less critical)
vm.watermark_scale_factor=100
```

### For Systems with Heavy Filesystem Activity

Unusual for network equipment, but may occur on systems with extensive logging or storage features.

```conf
# Allow more dirty pages for better write batching
vm.dirty_ratio=20
vm.dirty_background_ratio=10

# Longer dirty page expiration for write coalescing
vm.dirty_expire_centisecs=3000
```

