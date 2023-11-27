#ifndef _ALDER_MPP_H
#define _ALDER_MPP_H

#define CP_SMI_MDIO(X)        X( "mpp0",   none, 0, 0)
#define CP_SMI_MDC(X)         X( "mpp1",   none, 0, 0)
#define CP_XSMI_MDIO(X)       X( "mpp2",   none, 0, 0)
#define CP_XSMI_MDC(X)        X( "mpp3",   none, 0, 0)
/*  mpp4: Unused */
/*  mpp5: Unused */
/*  mpp6: Unused */
#define SFP9_TX_FAULT(X)      X( "mpp7", cp0_gpio1,  7, GPIO_ACTIVE_HIGH)
#define SFP9_TX_DISABLE(X)    X( "mpp8", cp0_gpio1,  8, GPIO_ACTIVE_HIGH)
#define SFP9_MOD_ABS(X)       X( "mpp9", cp0_gpio1,  9, GPIO_ACTIVE_LOW)
#define SW_RESETn(X)          X("mpp10", cp0_gpio1, 10, GPIO_ACTIVE_LOW)
#define SFP9_RS0(X)           X("mpp11", cp0_gpio1, 11, GPIO_ACTIVE_HIGH)
/* mpp12: Unused */
#define CP_SPI1_MISO(X)       X("mpp13", none, 0, 0)
#define CP_SPI1_CS0(X)        X("mpp14", none, 0, 0)
#define CP_SPI1_MOSI(X)       X("mpp15", none, 0, 0)
#define CP_SPI1_SCK(X)        X("mpp16", none, 0, 0)
#define WDT_TICKLE(X)         X("mpp17", cp0_gpio1, 17, GPIO_ACTIVE_HIGH)
/* mpp18: Unused */
/* mpp19: Unused */
/* mpp20: Unused */
/* mpp21: Unused */
/* mpp22: Unused */
/* mpp23: Unused */
#define DDR_TEN(X)            X("mpp24", cp0_gpio1, 24, GPIO_ACTIVE_HIGH)
#define ETH9_RESETn(X)        X("mpp25", cp0_gpio1, 25, GPIO_ACTIVE_LOW)
#define SW_INTn(X)            X("mpp26", cp0_gpio1, 26, IRQ_TYPE_LEVEL_LOW)
#define SFP9_RS1(X)           X("mpp27", cp0_gpio1, 27, GPIO_ACTIVE_HIGH)
#define SFP10_TX_FAULT(X)     X("mpp28", cp0_gpio1, 28, GPIO_ACTIVE_HIGH)
#define CP_UA0_RXD(X)         X("mpp29", none, 0, 0)
#define CP_UA0_TXD(X)         X("mpp30", none, 0, 0)
#define SFP10_TX_DISABLE(X)   X("mpp31", cp0_gpio1, 31, GPIO_ACTIVE_HIGH)
#define SFP10_MOD_ABS(X)      X("mpp32", cp0_gpio2,  0, GPIO_ACTIVE_LOW)
#define I2C_IRQ(X)            X("mpp33", cp0_gpio2,  1, IRQ_TYPE_LEVEL_LOW)
#define SFP10_RS0(X)          X("mpp34", cp0_gpio2,  2, GPIO_ACTIVE_HIGH)
#define CP_I2C1_SDA(X)        X("mpp35", none, 0, 0)
#define CP_I2C1_SCK(X)        X("mpp36", none, 0, 0)
#define CP_I2C0_SCK(X)        X("mpp37", none, 0, 0)
#define CP_I2C0_SDA(X)        X("mpp38", none, 0, 0)
#define SFP10_RX_LOS(X)       X("mpp39", cp0_gpio2,  7, GPIO_ACTIVE_HIGH)
#define SFP10_RS1(X)          X("mpp40", cp0_gpio2,  8, GPIO_ACTIVE_HIGH)
#define CP_SD_CRD_PWR_OFF(X)  X("mpp41", none,  0, 0)
#define CP_SD_HST_18_EN(X)    X("mpp42", none,  0, 0)
#define CP_SD_CRD_DT(X)       X("mpp43", none,  0, 0)
#define ETH9_INTn(X)          X("mpp44", cp0_gpio2, 12, GPIO_ACTIVE_LOW)
/* mpp45: Unused */
#define ETH10_RESETn(X)       X("mpp46", cp0_gpio2, 14, GPIO_ACTIVE_LOW)
#define I2C_RESETn(X)         X("mpp47", cp0_gpio2, 15, GPIO_ACTIVE_LOW)
#define ETH10_INTn(X)         X("mpp48", cp0_gpio2, 16, GPIO_ACTIVE_LOW)
#define DEV_MODEn(X)          X("mpp49", cp0_gpio2, 17, GPIO_ACTIVE_LOW)
#define USB1_VBUS_ENABLE(X)   X("mpp50", cp0_gpio2, 18, GPIO_ACTIVE_HIGH)
#define USB1_VBUS_ERROR_OC(X) X("mpp51", cp0_gpio2, 19, GPIO_ACTIVE_HIGH)
/* mpp52: Unused */
#define SFP9_RX_LOS(X)        X("mpp53", cp0_gpio2, 21, GPIO_ACTIVE_HIGH)
/* mpp54: Unused */
#define CP_SD_LED(X)          X("mpp55", none,  0, 0)
#define CP_SD_CLK(X)          X("mpp56", none,  0, 0)
#define CP_SD_CMD(X)          X("mpp57", none,  0, 0)
#define CP_SD_D0(X)           X("mpp58", none,  0, 0)
#define CP_SD_D1(X)           X("mpp59", none,  0, 0)
#define CP_SD_D2(X)           X("mpp60", none,  0, 0)
#define CP_SD_D3(X)           X("mpp61", none,  0, 0)
/* mpp62: Unused */

/* Macros to extract MPP info in different formats */
#define MPP_ID(_mpp, _chip, _no, _flags) _mpp
#define MPP_GPIO_CHIP(_mpp, _chip, _no, _flags) _chip

#define MPP_GPIO_REF(_mpp, _chip, _no, _flags) <&_chip _no _flags>
#define MPP_GPIO_REF_NO_CHIP(_mpp, _chip, _no, _flags) <_no _flags>
#define MPP_IRQ_REF(_mpp, _chip, _no, _flags) <&_chip _no _flags>

#endif	/* _ALDER_MPP_H */
