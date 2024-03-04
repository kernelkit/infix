#ifndef _STYX_MPP_H
#define _STYX_MPP_H

#define SFP0_TX_DISABLE(X) X( "mpp0", cp0_gpio1,  0, GPIO_ACTIVE_HIGH | GPIO_OPEN_DRAIN)
#define SFP1_TX_DISABLE(X) X( "mpp1", cp0_gpio1,  1, GPIO_ACTIVE_HIGH | GPIO_OPEN_DRAIN)
#define SFP2_TX_DISABLE(X) X( "mpp2", cp0_gpio1,  2, GPIO_ACTIVE_HIGH | GPIO_OPEN_DRAIN)
#define SFP3_TX_DISABLE(X) X( "mpp3", cp0_gpio1,  3, GPIO_ACTIVE_HIGH | GPIO_OPEN_DRAIN)
#define SFP0_RS0(X)        X( "mpp4", cp0_gpio1,  4, GPIO_ACTIVE_HIGH)
#define SFP1_RS0(X)        X( "mpp5", cp0_gpio1,  5, GPIO_ACTIVE_HIGH)
#define SFP2_RS0(X)        X( "mpp6", cp0_gpio1,  6, GPIO_ACTIVE_HIGH)
#define SFP3_RS0(X)        X( "mpp7", cp0_gpio1,  7, GPIO_ACTIVE_HIGH)
#define SFP0_RS1(X)        X( "mpp8", cp0_gpio1,  8, GPIO_ACTIVE_HIGH)
#define SFP1_RS1(X)        X( "mpp9", cp0_gpio1,  9, GPIO_ACTIVE_HIGH)
#define SFP2_RS1(X)        X("mpp10", cp0_gpio1, 10, GPIO_ACTIVE_HIGH)
#define SFP3_RS1(X)        X("mpp11", cp0_gpio1, 11, GPIO_ACTIVE_HIGH)
/* mpp12: Unused */
#define CP_SPI1_MISO(X)    X("mpp13", none, 0, 0)
#define CP_SPI1_CS0(X)     X("mpp14", none, 0, 0)
#define CP_SPI1_MOSI(X)    X("mpp15", none, 0, 0)
#define CP_SPI1_SCK(X)     X("mpp16", none, 0, 0)
/* mpp17: Unused */
/* mpp18: Unused */
/* mpp19: Unused */
/* mpp20: Unused */
/* mpp21: Unused */
/* mpp22: Unused */
/* mpp23: Unused */
/* mpp24: Unused */
/* mpp25: Unused */
/* mpp26: Unused */
#define SFP0_RX_LOS(X)     X("mpp27", cp0_gpio1, 27, GPIO_ACTIVE_HIGH)
#define SFP1_RX_LOS(X)     X("mpp28", cp0_gpio1, 28, GPIO_ACTIVE_HIGH)
#define SFP2_RX_LOS(X)     X("mpp29", cp0_gpio1, 29, GPIO_ACTIVE_HIGH)
#define SFP3_RX_LOS(X)     X("mpp30", cp0_gpio1, 30, GPIO_ACTIVE_HIGH)
#define SFP0_TX_FAULT(X)   X("mpp31", cp0_gpio1, 31, GPIO_ACTIVE_HIGH)
#define SFP1_TX_FAULT(X)   X("mpp32", cp0_gpio2,  0, GPIO_ACTIVE_HIGH)
#define SFP2_TX_FAULT(X)   X("mpp33", cp0_gpio2,  1, GPIO_ACTIVE_HIGH)
#define SFP3_TX_FAULT(X)   X("mpp34", cp0_gpio2,  2, GPIO_ACTIVE_HIGH)
#define CP_I2C1_SDA(X)     X("mpp35", none, 0, 0)
#define CP_I2C1_SCK(X)     X("mpp36", none, 0, 0)
#define CP_I2C0_SCK(X)     X("mpp37", none, 0, 0)
#define CP_I2C0_SDA(X)     X("mpp38", none, 0, 0)
/* mpp39: Unused */
#define CP_SMI_MDIO(X)     X("mpp40",   none, 0, 0)
#define CP_SMI_MDC(X)      X("mpp41",   none, 0, 0)
/* mpp42: Unused */
/* mpp43: Unused */
/* mpp44: Unused */
/* mpp45: Unused */
/* mpp46: Unused */
#define CP_UA1_TXD(X)      X("mpp47", none, 0, 0)
/* mpp48: Unused */
#define SW1_RESETn(X)      X("mpp49", cp0_gpio2, 17, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
#define SW2_RESETn(X)      X("mpp50", cp0_gpio2, 18, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
#define SW3_RESETn(X)      X("mpp51", cp0_gpio2, 19, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
/* mpp52: Unused */
#define CP_UA1_RXD(X)      X("mpp53", none, 0, 0)
#define SFP0_MOD_ABS(X)    X("mpp54", cp0_gpio2, 22, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
#define SFP1_MOD_ABS(X)    X("mpp55", cp0_gpio2, 23, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
#define SFP2_MOD_ABS(X)    X("mpp56", cp0_gpio2, 24, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
#define SFP3_MOD_ABS(X)    X("mpp57", cp0_gpio2, 25, GPIO_ACTIVE_LOW | GPIO_OPEN_DRAIN)
#define SW1_INTn(X)        X("mpp58", cp0_gpio2, 26, IRQ_TYPE_LEVEL_LOW)
#define SW2_INTn(X)        X("mpp59", cp0_gpio2, 27, IRQ_TYPE_LEVEL_LOW)
#define SW3_INTn(X)        X("mpp60", cp0_gpio2, 28, IRQ_TYPE_LEVEL_LOW)
/* mpp61: Unused */
#define DDR_TEN(X)         X("mpp62", cp0_gpio2, 30, GPIO_ACTIVE_HIGH)

/* Macros to extract MPP info in different formats */
#define MPP_ID(_mpp, _chip, _no, _flags) _mpp
#define MPP_GPIO_CHIP(_mpp, _chip, _no, _flags) _chip

#define MPP_GPIO_REF(_mpp, _chip, _no, _flags) <&_chip _no (_flags)>
#define MPP_GPIO_REF_NO_CHIP(_mpp, _chip, _no, _flags) <_no (_flags)>
#define MPP_IRQ_REF(_mpp, _chip, _no, _flags) <&_chip _no (_flags)>

#endif	/* _STYX_MPP_H */
