// Code generated from C++行情SDK文件/hsds_comudata.h; DO NOT EDIT.
package quoteprotocol

const (
	RT_RISE                         uint16 = 0x0001
	RT_Srv_Sub_Restart              uint16 = 0x0001
	RT_FALL                         uint16 = 0x0002
	RT_Srv_Sub_Replace              uint16 = 0x0002
	RT_Srv_Sub_DownCFG              uint16 = 0x0003
	RT_5_RISE                       uint16 = 0x0004
	RT_Srv_Sub_UpCFG                uint16 = 0x0004
	RT_Srv_Sub_DownUserDB           uint16 = 0x0005
	RT_Srv_Sub_UpUserDB             uint16 = 0x0006
	RT_Srv_Sub_DownReport           uint16 = 0x0007
	RT_5_FALL                       uint16 = 0x0008
	RT_Srv_Sub_LimitPrompt          uint16 = 0x0008
	RT_AHEAD_COMM                   uint16 = 0x0010
	RT_JAVA_MARK                    uint16 = 0x0010
	RT_AFTER_COMM                   uint16 = 0x0020
	RT_WINCE_MARK                   uint16 = 0x0020
	RT_AHEAD_PRICE                  uint16 = 0x0040
	RT_NOTZIP                       uint16 = 0x0040
	RT_AHEAD_VOLBI                  uint16 = 0x0080
	RT_DEBUG                        uint16 = 0x0080
	RT_AHEAD_MONEY                  uint16 = 0x0100
	RT_BEGIN                        uint16 = 0x0100
	RT_INITIALINFO                  uint16 = 0x0101
	RT_LOGIN                        uint16 = 0x0102
	RT_SERVERINFO                   uint16 = 0x0103
	RT_BULLETIN                     uint16 = 0x0104
	RT_PARTINITIALINFO              uint16 = 0x0105
	RT_ETF_INIT                     uint16 = 0x0106
	RT_LOGIN_HK                     uint16 = 0x0107
	RT_LOGIN_FUTURES                uint16 = 0x0108
	RT_LOGIN_FOREIGN                uint16 = 0x0109
	RT_LOGIN_WP                     uint16 = 0x010A
	RT_LOGIN_INFO                   uint16 = 0x010B
	RT_CHANGE_PWD                   uint16 = 0x010C
	RT_LOGIN_LEVEL                  uint16 = 0x010D
	RT_SERVERINFO2                  uint16 = 0x010E
	RT_VERIFYINFO                   uint16 = 0x010F
	RT_LOGIN_CENTER                 uint16 = 0x0110
	RT_LOGIN_FTP                    uint16 = 0x0111
	RT_MODIFY_LV2PWD                uint16 = 0x0112
	RT_MODIFY_PWD                   uint16 = 0x0112
	RT_DISCONNLEVEL2                uint16 = 0x0113
	RT_STOCKINFOLIST                uint16 = 0x0114
	RT_LOGIN_COMMODITIES            uint16 = 0x0120
	RT_LOGIN_CAE                    uint16 = 0x0123
	RT_LOGIN_US                     uint16 = 0x0124
	RT_LOGIN_OPTION                 uint16 = 0x0125
	RT_INITIALINFO_VARCODE          uint16 = 0x0134
	RT_REALTIME                     uint16 = 0x0201
	RT_DYNREPORT                    uint16 = 0x0202
	RT_REPORTSORT                   uint16 = 0x0203
	RT_GENERALSORT                  uint16 = 0x0204
	RT_GENERALSORT_EX               uint16 = 0x0205
	RT_SEVER_EMPTY                  uint16 = 0x0206
	RT_SEVER_CALCULATE              uint16 = 0x0207
	RT_ANS_BYTYPE                   uint16 = 0x0208
	RT_QHMM_REALTIME                uint16 = 0x0209
	RT_LEVEL_REALTIME               uint16 = 0x020A
	RT_CLASS_REALTIME               uint16 = 0x020B
	RT_LEVEL_ORDERQUEUE             uint16 = 0x020C
	RT_LEVEL_TRANSACTION            uint16 = 0x020D
	RT_QHARB_REALTIME               uint16 = 0x020E
	RT_REALTIME_EXT                 uint16 = 0x020F
	RT_REPORTSORT_EXT               uint16 = 0x0212
	RT_HQCOL_VALUE                  uint16 = 0x0216
	RT_MARKETSORT                   uint16 = 0x0217
	RT_ASSIST_QUERY                 uint16 = 0x0218
	RT_DYNAUCTION_REALTIME          uint16 = 0x0219
	RT_DYNAUCTION_TICK              uint16 = 0x021A
	RT_GET_TABLE_ALLDATA            uint16 = 0x021B
	RT_FIXED_PRICE_REALTIME         uint16 = 0x021C
	RT_GET_RELAVENT_BLOCK           uint16 = 0x021D
	RT_REALTIME_SENIOR              uint16 = 0x022A
	RT_TREND                        uint16 = 0x0301
	RT_ADDTREND                     uint16 = 0x0302
	RT_BUYSELLPOWER                 uint16 = 0x0303
	RT_HISTREND                     uint16 = 0x0304
	RT_TICK                         uint16 = 0x0305
	RT_ETF_TREND                    uint16 = 0x0306
	RT_ETF_NOWDATA                  uint16 = 0x0307
	RT_ETF_TREND_TECH               uint16 = 0x0308
	RT_HISTREND_INDEX               uint16 = 0x0309
	RT_AUTOARBPUSH                  uint16 = 0x030A
	RT_TREND_EXT                    uint16 = 0x030B
	RT_HISTREND_EXT                 uint16 = 0x030C
	RT_VIRTUAL_AUCTION              uint16 = 0x030E
	RT_LEVEL2_SHZB                  uint16 = 0x03E9
	RT_LEVEL2_SHWTDL                uint16 = 0x03EA
	RT_LEVEL2_SHKZ                  uint16 = 0x03EC
	RT_LEVEL2_SHKZ_PH               uint16 = 0x03F2
	RT_TECHDATA                     uint16 = 0x0400
	RT_FILEDOWNLOAD                 uint16 = 0x0401
	RT_TECHDATA_EX                  uint16 = 0x0402
	RT_DATA_WEIHU                   uint16 = 0x0403
	RT_FILEDOWNLOAD2                uint16 = 0x0404
	RT_FILED_CFG                    uint16 = 0x0405
	RT_FILL_DATA                    uint16 = 0x0406
	RT_TECHDATA_BYPERIOD            uint16 = 0x0407
	RT_ONE_BLOCKDATA                uint16 = 0x0408
	RT_TECHDATA_INCREMENT           uint16 = 0x0409
	RT_TECHDATA_RANGE               uint16 = 0x0410
	RT_TECHDATA_BIGINT              uint16 = 0x0411
	RT_TECHDATA_RANGE_BIGINT        uint16 = 0x0412
	RT_TECHDATA_AFTER               uint16 = 0x0413
	RT_TEXTDATAWITHINDEX_PLUS       uint16 = 0x0501
	RT_TEXTDATAWITHINDEX_NEGATIVE   uint16 = 0x0502
	RT_BYINDEXRETDATA               uint16 = 0x0503
	RT_USERTEXTDATA                 uint16 = 0x0504
	RT_FILEREQUEST                  uint16 = 0x0505
	RT_FILESimplify                 uint16 = 0x0506
	RT_ATTATCHDATA                  uint16 = 0x0507
	RT_PROMPT_INFO                  uint16 = 0x0508
	RT_STOCKTICK                    uint16 = 0x0601
	RT_BUYSELLORDER                 uint16 = 0x0602
	RT_LIMITTICK                    uint16 = 0x0603
	RT_HISTORYTICK                  uint16 = 0x0604
	RT_MAJORINDEXTICK               uint16 = 0x0605
	RT_VALUE                        uint16 = 0x0606
	RT_BUYSELLORDER_HK              uint16 = 0x0607
	RT_BUYSELLORDER_FUTURES         uint16 = 0x0608
	RT_VALUE_HK                     uint16 = 0x0609
	RT_VALUE_FUTURES                uint16 = 0x060A
	RT_TOTAL                        uint16 = 0x060B
	RT_LIMITTICK_EX                 uint16 = 0x060C
	RT_MINUTE_TICK                  uint16 = 0x0612
	RT_AFTER_TICK                   uint16 = 0x0617
	RT_LEAD                         uint16 = 0x0702
	RT_MAJORINDEXTREND              uint16 = 0x0703
	RT_MAJORINDEXADL                uint16 = 0x0704
	RT_MAJORINDEXDBBI               uint16 = 0x0705
	RT_MAJORINDEXBUYSELL            uint16 = 0x0706
	RT_SERVERFILEINFO               uint16 = 0x0707
	RT_DOWNSERVERFILEINFO           uint16 = 0x0708
	RT_LEVEL2_SZZB                  uint16 = 0x07D1
	RT_LEVEL2_SZWTDL                uint16 = 0x07D2
	RT_LEVEL2_SZKZ                  uint16 = 0x07D4
	RT_LEVEL2_SZKZ_PH               uint16 = 0x07DB
	RT_LEVEL2_SZKZ_TD               uint16 = 0x07DE
	RT_CURRENTFINANCEDATA           uint16 = 0x0801
	RT_HISFINANCEDATA               uint16 = 0x0802
	RT_EXRIGHT_DATA                 uint16 = 0x0803
	RT_HK_RECORDOPTION              uint16 = 0x0804
	RT_BROKER_HK                    uint16 = 0x0805
	RT_BLOCK_DATA                   uint16 = 0x0806
	RT_STATIC_HK                    uint16 = 0x0807
	RT_USER_BLOCK_DATA              uint16 = 0x0810
	RT_HK_SH_BREAKER_VCM            uint16 = 0x0812
	RT_HK_SH_BREAKER_CAS            uint16 = 0x0814
	RT_MASSDATA                     uint16 = 0x0901
	RT_SERVERTIME                   uint16 = 0x0902
	RT_KEEPACTIVE                   uint16 = 0x0903
	RT_TEST                         uint16 = 0x0904
	RT_TESTSRV                      uint16 = 0x0905
	RT_PUSHINFODATA                 uint16 = 0x0906
	RT_MARKET_MONITOR_SUPPORT_QUERY uint16 = 0x0907
	RT_MARKET_MONITOR_STOCK_QUERY   uint16 = 0x0908
	RT_MARKET_MONITOR_DATE_QUERY    uint16 = 0x0909
	RT_AUTOPUSH                     uint16 = 0x0A01
	RT_AUTOPUSHSIMP                 uint16 = 0x0A02
	RT_REQAUTOPUSH                  uint16 = 0x0A03
	RT_ETF_AUTOPUSH                 uint16 = 0x0A04
	RT_AUTOBROKER_HK                uint16 = 0x0A05
	RT_AUTOTICK_HK                  uint16 = 0x0A06
	RT_AUTOPUSH_QH                  uint16 = 0x0A07
	RT_PUSHREALTIMEINFO             uint16 = 0x0A08
	RT_RAW_AUTOPUSH                 uint16 = 0x0A09
	RT_QHMM_AUTOPUSH                uint16 = 0x0A0A
	RT_LEVEL_AUTOPUSH               uint16 = 0x0A0B
	RT_LEVEL_BORDERQUEUE_AUTOPUSH   uint16 = 0x0A0C
	RT_LEVEL_SORDERQUEUE_AUTOPUSH   uint16 = 0x0A0D
	RT_LEVEL_TRANSACTION_AUTOPUSH   uint16 = 0x0A0E
	RT_AUTOPUSH_EXT                 uint16 = 0x0A0F
	RT_AUTO_TOTALMAX                uint16 = 0x0A10
	RT_AUTO_SINGLEMAX               uint16 = 0x0A11
	RT_AUTOPUSH_RES                 uint16 = 0x0A12
	RT_AUTOPUSH_LV2RES              uint16 = 0x0A13
	RT_MARKET_MONITOR_AUTOPUSH      uint16 = 0x0A17
	RT_DYNAUCTION_AUTOPUSH          uint16 = 0x0A18
	RT_FIXPRICE_AUTOPUSH            uint16 = 0x0A1A
	RT_HK_SH_BREAKER_VCM_AUTOPUSH   uint16 = 0x0A29
	RT_HK_SH_BREAKER_CAS_AUTOPUSH   uint16 = 0x0A30
	RT_AUTOPUSH_SENIOR              uint16 = 0x0A32
	RT_UPDATEDFINANCIALDATA         uint16 = 0x0B01
	RT_SYNCHRONIZATIONDATA          uint16 = 0x0B02
	RT_MARKET_NOTICE                uint16 = 0x0B03
	RT_Send_Notice                  uint16 = 0x0C01
	RT_Send_ScrollText              uint16 = 0x0C02
	RT_Change_program               uint16 = 0x0C03
	RT_Send_File_Data               uint16 = 0x0C04
	RT_RequestDBF                   uint16 = 0x0C05
	RT_InfoSend                     uint16 = 0x0C06
	RT_InfoUpdateIndex              uint16 = 0x0C07
	RT_InfoUpdateOneIndex           uint16 = 0x0C08
	RT_NoteMsgData                  uint16 = 0x0C09
	RT_InfoDataTransmit             uint16 = 0x0C0A
	RT_InfoCheckPurview             uint16 = 0x0C0B
	RT_InfoClickTime                uint16 = 0x0C0C
	RT_REPORTSORT_Simple            uint16 = 0x0D01
	RT_PARTINITIALINFO_Simple       uint16 = 0x0D02
	RT_RETURN_EMPTY                 uint16 = 0x0D03
	RT_InfoDataRailing              uint16 = 0x0D04
	RT_Srv_SrvStatus                uint16 = 0x0F01
	RT_SRV_SYNC                     uint16 = 0x0F02
	RT_Srv_Sub_Succ                 uint16 = 0x1000
	RT_REPORTSORT_INT64             uint16 = 0x1203
	RT_GENERALSORT_EX_INT64         uint16 = 0x1205
	RT_LEVEL_REALTIME_INT64         uint16 = 0x120A
	RT_REALTIME_EXT_INT64           uint16 = 0x120F
	RT_MARKETSORT_INT64             uint16 = 0x1217
	RT_TREND_INT64                  uint16 = 0x1301
	RT_ADDTREND_INT64               uint16 = 0x1302
	RT_BUYSELLPOWER_INT64           uint16 = 0x1303
	RT_HISTREND_INT64               uint16 = 0x1304
	RT_TICK_INT64                   uint16 = 0x1305
	RT_HISTREND_INDEX_INT64         uint16 = 0x1309
	RT_TREND_EXT_INT64              uint16 = 0x130B
	RT_STOCKTICK_INT64              uint16 = 0x1601
	RT_LIMITTICK_INT64              uint16 = 0x1603
	RT_MAJORINDEXTICK_INT64         uint16 = 0x1605
	RT_VALUE_INT64                  uint16 = 0x1606
	RT_STOCKTICK_SEARCH_INT64       uint16 = 0x160C
	RT_MINUTE_TICK_INT64            uint16 = 0x1612
	RT_LEAD_INT64                   uint16 = 0x1702
	RT_MAJORINDEXTREND_INT64        uint16 = 0x1703
	RT_LEVEL_AUTOPUSH_INT64         uint16 = 0x1A0B
	RT_AUTOPUSH_EXT_INT64           uint16 = 0x1A0F
	RT_LEVEL_TOTALMAX               uint16 = 0x2001
	RT_LEVEL_SINGLEMAX              uint16 = 0x2002
	RT_ZIPDATA                      uint16 = 0x8001
	RT_COMPASKDATA                  uint16 = 0x8FFE
	RT_END                          uint16 = 0x8FFF
	RT_LEVEL2_SHWTDL_TD             uint16 = 0xF3EA
	RT_LEVEL2_SHKZ_TD               uint16 = 0xF3EC
	RT_LEVEL2_SHZB_TD               uint16 = 0xF3F9
	RT_LEVEL2_SZZB_TD               uint16 = 0xF7D1
	RT_LEVEL2_SZWTDL_TD             uint16 = 0xF7D2
)

type FunctionDefinition struct {
	Name       string `json:"name"`
	Value      uint16 `json:"value"`
	Hex        string `json:"hex"`
	Comment    string `json:"comment,omitempty"`
	SourceLine int    `json:"source_line"`
}

var functionDefinitions = []FunctionDefinition{
	{Name: "RT_RISE", Value: RT_RISE, Hex: "0x0001", Comment: "涨幅排名", SourceLine: 408},
	{Name: "RT_Srv_Sub_Restart", Value: RT_Srv_Sub_Restart, Hex: "0x0001", Comment: "重新启动程序", SourceLine: 324},
	{Name: "RT_FALL", Value: RT_FALL, Hex: "0x0002", Comment: "跌幅排名", SourceLine: 409},
	{Name: "RT_Srv_Sub_Replace", Value: RT_Srv_Sub_Replace, Hex: "0x0002", Comment: "替换程序", SourceLine: 325},
	{Name: "RT_Srv_Sub_DownCFG", Value: RT_Srv_Sub_DownCFG, Hex: "0x0003", Comment: "下载配置文件", SourceLine: 327},
	{Name: "RT_5_RISE", Value: RT_5_RISE, Hex: "0x0004", Comment: "5分钟涨幅排名", SourceLine: 410},
	{Name: "RT_Srv_Sub_UpCFG", Value: RT_Srv_Sub_UpCFG, Hex: "0x0004", Comment: "上传配置文件", SourceLine: 328},
	{Name: "RT_Srv_Sub_DownUserDB", Value: RT_Srv_Sub_DownUserDB, Hex: "0x0005", Comment: "下载用户管理文件文件", SourceLine: 330},
	{Name: "RT_Srv_Sub_UpUserDB", Value: RT_Srv_Sub_UpUserDB, Hex: "0x0006", Comment: "上传用户管理文件文件", SourceLine: 331},
	{Name: "RT_Srv_Sub_DownReport", Value: RT_Srv_Sub_DownReport, Hex: "0x0007", Comment: "下载后台程序报告文件", SourceLine: 333},
	{Name: "RT_5_FALL", Value: RT_5_FALL, Hex: "0x0008", Comment: "5分钟跌幅排名", SourceLine: 411},
	{Name: "RT_Srv_Sub_LimitPrompt", Value: RT_Srv_Sub_LimitPrompt, Hex: "0x0008", Comment: "权限错误提示", SourceLine: 334},
	{Name: "RT_AHEAD_COMM", Value: RT_AHEAD_COMM, Hex: "0x0010", Comment: "买卖量差(委比)正序排名", SourceLine: 412},
	{Name: "RT_JAVA_MARK", Value: RT_JAVA_MARK, Hex: "0x0010", Comment: "JAVA登录 | RT_LOGIN_*", SourceLine: 54},
	{Name: "RT_AFTER_COMM", Value: RT_AFTER_COMM, Hex: "0x0020", Comment: "买卖量差(委比)倒序排名", SourceLine: 413},
	{Name: "RT_WINCE_MARK", Value: RT_WINCE_MARK, Hex: "0x0020", Comment: "WINCE登录 | RT_LOGIN_*", SourceLine: 55},
	{Name: "RT_AHEAD_PRICE", Value: RT_AHEAD_PRICE, Hex: "0x0040", Comment: "成交价震幅正序排名", SourceLine: 414},
	{Name: "RT_NOTZIP", Value: RT_NOTZIP, Hex: "0x0040", Comment: "不使用压缩数据传输", SourceLine: 56},
	{Name: "RT_AHEAD_VOLBI", Value: RT_AHEAD_VOLBI, Hex: "0x0080", Comment: "成交量变化(量比)正序排名", SourceLine: 415},
	{Name: "RT_DEBUG", Value: RT_DEBUG, Hex: "0x0080", Comment: "不使用压缩数据传输", SourceLine: 57},
	{Name: "RT_AHEAD_MONEY", Value: RT_AHEAD_MONEY, Hex: "0x0100", Comment: "资金流向正序排名", SourceLine: 416},
	{Name: "RT_BEGIN", Value: RT_BEGIN, Hex: "0x0100", Comment: "", SourceLine: 48},
	{Name: "RT_INITIALINFO", Value: RT_INITIALINFO, Hex: "0x0101", Comment: "客户端数据所有初始化", SourceLine: 59},
	{Name: "RT_LOGIN", Value: RT_LOGIN, Hex: "0x0102", Comment: "客户端登录行情服务器", SourceLine: 60},
	{Name: "RT_SERVERINFO", Value: RT_SERVERINFO, Hex: "0x0103", Comment: "主站信息", SourceLine: 61},
	{Name: "RT_BULLETIN", Value: RT_BULLETIN, Hex: "0x0104", Comment: "紧急公告(主推)", SourceLine: 62},
	{Name: "RT_PARTINITIALINFO", Value: RT_PARTINITIALINFO, Hex: "0x0105", Comment: "客户端数据部分初始化", SourceLine: 63},
	{Name: "RT_ETF_INIT", Value: RT_ETF_INIT, Hex: "0x0106", Comment: "52只股票数据(ETF)", SourceLine: 64},
	{Name: "RT_LOGIN_HK", Value: RT_LOGIN_HK, Hex: "0x0107", Comment: "客户端登录港股服务器", SourceLine: 65},
	{Name: "RT_LOGIN_FUTURES", Value: RT_LOGIN_FUTURES, Hex: "0x0108", Comment: "客户端登录期货服务器", SourceLine: 66},
	{Name: "RT_LOGIN_FOREIGN", Value: RT_LOGIN_FOREIGN, Hex: "0x0109", Comment: "客户端登录外汇服务器", SourceLine: 67},
	{Name: "RT_LOGIN_WP", Value: RT_LOGIN_WP, Hex: "0x010A", Comment: "客户端登录外盘服务器", SourceLine: 68},
	{Name: "RT_LOGIN_INFO", Value: RT_LOGIN_INFO, Hex: "0x010B", Comment: "客户端登录资讯服务器", SourceLine: 69},
	{Name: "RT_CHANGE_PWD", Value: RT_CHANGE_PWD, Hex: "0x010C", Comment: "修改密码", SourceLine: 70},
	{Name: "RT_LOGIN_LEVEL", Value: RT_LOGIN_LEVEL, Hex: "0x010D", Comment: "level2登陆请求", SourceLine: 72},
	{Name: "RT_SERVERINFO2", Value: RT_SERVERINFO2, Hex: "0x010E", Comment: "主站信息2", SourceLine: 73},
	{Name: "RT_VERIFYINFO", Value: RT_VERIFYINFO, Hex: "0x010F", Comment: "用户认证信息返回", SourceLine: 74},
	{Name: "RT_LOGIN_CENTER", Value: RT_LOGIN_CENTER, Hex: "0x0110", Comment: "center登陆请求", SourceLine: 76},
	{Name: "RT_LOGIN_FTP", Value: RT_LOGIN_FTP, Hex: "0x0111", Comment: "FTP登录返回请求", SourceLine: 78},
	{Name: "RT_MODIFY_LV2PWD", Value: RT_MODIFY_LV2PWD, Hex: "0x0112", Comment: "修改Level2用户密码", SourceLine: 80},
	{Name: "RT_MODIFY_PWD", Value: RT_MODIFY_PWD, Hex: "0x0112", Comment: "修改密码", SourceLine: 79},
	{Name: "RT_DISCONNLEVEL2", Value: RT_DISCONNLEVEL2, Hex: "0x0113", Comment: "level2断线", SourceLine: 81},
	{Name: "RT_STOCKINFOLIST", Value: RT_STOCKINFOLIST, Hex: "0x0114", Comment: "请求代码列表", SourceLine: 82},
	{Name: "RT_LOGIN_COMMODITIES", Value: RT_LOGIN_COMMODITIES, Hex: "0x0120", Comment: "商品现货 包含黄金", SourceLine: 84},
	{Name: "RT_LOGIN_CAE", Value: RT_LOGIN_CAE, Hex: "0x0123", Comment: "文交所登录", SourceLine: 85},
	{Name: "RT_LOGIN_US", Value: RT_LOGIN_US, Hex: "0x0124", Comment: "美股登陆", SourceLine: 86},
	{Name: "RT_LOGIN_OPTION", Value: RT_LOGIN_OPTION, Hex: "0x0125", Comment: "期权登录", SourceLine: 87},
	{Name: "RT_INITIALINFO_VARCODE", Value: RT_INITIALINFO_VARCODE, Hex: "0x0134", Comment: "变长代码的初始化信息", SourceLine: 88},
	{Name: "RT_REALTIME", Value: RT_REALTIME, Hex: "0x0201", Comment: "行情报价表:1-6乾隆操作键", SourceLine: 90},
	{Name: "RT_DYNREPORT", Value: RT_DYNREPORT, Hex: "0x0202", Comment: "强弱分析;指标排序;热门板块分析;区间分析;跑马灯股票列表数据;预警", SourceLine: 91},
	{Name: "RT_REPORTSORT", Value: RT_REPORTSORT, Hex: "0x0203", Comment: "排名报价表:61-66、点击列排序", SourceLine: 92},
	{Name: "RT_GENERALSORT", Value: RT_GENERALSORT, Hex: "0x0204", Comment: "综合排名报表:81-86", SourceLine: 93},
	{Name: "RT_GENERALSORT_EX", Value: RT_GENERALSORT_EX, Hex: "0x0205", Comment: "综合排名报表:81-86加入自定义分钟排名", SourceLine: 94},
	{Name: "RT_SEVER_EMPTY", Value: RT_SEVER_EMPTY, Hex: "0x0206", Comment: "服务器无数据返回空包", SourceLine: 95},
	{Name: "RT_SEVER_CALCULATE", Value: RT_SEVER_CALCULATE, Hex: "0x0207", Comment: "服务器计算数据包,包括涨停、跌停", SourceLine: 96},
	{Name: "RT_ANS_BYTYPE", Value: RT_ANS_BYTYPE, Hex: "0x0208", Comment: "根据类型返回数据包", SourceLine: 97},
	{Name: "RT_QHMM_REALTIME", Value: RT_QHMM_REALTIME, Hex: "0x0209", Comment: "期货买卖盘", SourceLine: 98},
	{Name: "RT_LEVEL_REALTIME", Value: RT_LEVEL_REALTIME, Hex: "0x020A", Comment: "level", SourceLine: 99},
	{Name: "RT_CLASS_REALTIME", Value: RT_CLASS_REALTIME, Hex: "0x020B", Comment: "根据分类类别获取行情报价", SourceLine: 100},
	{Name: "RT_LEVEL_ORDERQUEUE", Value: RT_LEVEL_ORDERQUEUE, Hex: "0x020C", Comment: "level2买卖队列", SourceLine: 102},
	{Name: "RT_LEVEL_TRANSACTION", Value: RT_LEVEL_TRANSACTION, Hex: "0x020D", Comment: "LEVEL2逐笔成交", SourceLine: 103},
	{Name: "RT_QHARB_REALTIME", Value: RT_QHARB_REALTIME, Hex: "0x020E", Comment: "期货套利报价 maxy add", SourceLine: 104},
	{Name: "RT_REALTIME_EXT", Value: RT_REALTIME_EXT, Hex: "0x020F", Comment: "", SourceLine: 231},
	{Name: "RT_REPORTSORT_EXT", Value: RT_REPORTSORT_EXT, Hex: "0x0212", Comment: "排名报价表扩展:61-66、点击列排序", SourceLine: 105},
	{Name: "RT_HQCOL_VALUE", Value: RT_HQCOL_VALUE, Hex: "0x0216", Comment: "按行情列ID查询行情字段值", SourceLine: 108},
	{Name: "RT_MARKETSORT", Value: RT_MARKETSORT, Hex: "0x0217", Comment: "按指定的行情列对市场排序", SourceLine: 109},
	{Name: "RT_ASSIST_QUERY", Value: RT_ASSIST_QUERY, Hex: "0x0218", Comment: "退市整理代码表", SourceLine: 111},
	{Name: "RT_DYNAUCTION_REALTIME", Value: RT_DYNAUCTION_REALTIME, Hex: "0x0219", Comment: "文交所动态竞价实时数据", SourceLine: 112},
	{Name: "RT_DYNAUCTION_TICK", Value: RT_DYNAUCTION_TICK, Hex: "0x021A", Comment: "文交所动态竞价Tick数据", SourceLine: 113},
	{Name: "RT_GET_TABLE_ALLDATA", Value: RT_GET_TABLE_ALLDATA, Hex: "0x021B", Comment: "请求获取全部的表单数据", SourceLine: 114},
	{Name: "RT_FIXED_PRICE_REALTIME", Value: RT_FIXED_PRICE_REALTIME, Hex: "0x021C", Comment: "定价交易实时报价", SourceLine: 115},
	{Name: "RT_GET_RELAVENT_BLOCK", Value: RT_GET_RELAVENT_BLOCK, Hex: "0x021D", Comment: "请求所属板块数据", SourceLine: 116},
	{Name: "RT_REALTIME_SENIOR", Value: RT_REALTIME_SENIOR, Hex: "0x022A", Comment: "行情多档快照查询数据功能号", SourceLine: 118},
	{Name: "RT_TREND", Value: RT_TREND, Hex: "0x0301", Comment: "分时走势", SourceLine: 121},
	{Name: "RT_ADDTREND", Value: RT_ADDTREND, Hex: "0x0302", Comment: "走势图叠加、多股同列", SourceLine: 122},
	{Name: "RT_BUYSELLPOWER", Value: RT_BUYSELLPOWER, Hex: "0x0303", Comment: "买卖力道", SourceLine: 123},
	{Name: "RT_HISTREND", Value: RT_HISTREND, Hex: "0x0304", Comment: "历史回忆;多日分时;右小图下分时走势", SourceLine: 124},
	{Name: "RT_TICK", Value: RT_TICK, Hex: "0x0305", Comment: "TICK图", SourceLine: 125},
	{Name: "RT_ETF_TREND", Value: RT_ETF_TREND, Hex: "0x0306", Comment: "ETF分时走势", SourceLine: 126},
	{Name: "RT_ETF_NOWDATA", Value: RT_ETF_NOWDATA, Hex: "0x0307", Comment: "ETF时时数据", SourceLine: 127},
	{Name: "RT_ETF_TREND_TECH", Value: RT_ETF_TREND_TECH, Hex: "0x0308", Comment: "ETFtech分时走势", SourceLine: 128},
	{Name: "RT_HISTREND_INDEX", Value: RT_HISTREND_INDEX, Hex: "0x0309", Comment: "对于大盘领先-历史回忆;多日分时;右小图下分时走势", SourceLine: 129},
	{Name: "RT_AUTOARBPUSH", Value: RT_AUTOARBPUSH, Hex: "0x030A", Comment: "期货套利报价 maxy add", SourceLine: 130},
	{Name: "RT_TREND_EXT", Value: RT_TREND_EXT, Hex: "0x030B", Comment: "", SourceLine: 230},
	{Name: "RT_HISTREND_EXT", Value: RT_HISTREND_EXT, Hex: "0x030C", Comment: "", SourceLine: 131},
	{Name: "RT_VIRTUAL_AUCTION", Value: RT_VIRTUAL_AUCTION, Hex: "0x030E", Comment: "查询集合竞价数据", SourceLine: 132},
	{Name: "RT_LEVEL2_SHZB", Value: RT_LEVEL2_SHZB, Hex: "0x03E9", Comment: "1001", SourceLine: 443},
	{Name: "RT_LEVEL2_SHWTDL", Value: RT_LEVEL2_SHWTDL, Hex: "0x03EA", Comment: "1002，上证委托队列", SourceLine: 451},
	{Name: "RT_LEVEL2_SHKZ", Value: RT_LEVEL2_SHKZ, Hex: "0x03EC", Comment: "1004", SourceLine: 447},
	{Name: "RT_LEVEL2_SHKZ_PH", Value: RT_LEVEL2_SHKZ_PH, Hex: "0x03F2", Comment: "1010, 上证盘后快照", SourceLine: 449},
	{Name: "RT_TECHDATA", Value: RT_TECHDATA, Hex: "0x0400", Comment: "盘后分析", SourceLine: 135},
	{Name: "RT_FILEDOWNLOAD", Value: RT_FILEDOWNLOAD, Hex: "0x0401", Comment: "文件请求（盘后数据下载）", SourceLine: 136},
	{Name: "RT_TECHDATA_EX", Value: RT_TECHDATA_EX, Hex: "0x0402", Comment: "盘后分析扩展 -- 支持基金净值", SourceLine: 137},
	{Name: "RT_DATA_WEIHU", Value: RT_DATA_WEIHU, Hex: "0x0403", Comment: "数据维护处理", SourceLine: 138},
	{Name: "RT_FILEDOWNLOAD2", Value: RT_FILEDOWNLOAD2, Hex: "0x0404", Comment: "下载服务器指定目录文件", SourceLine: 139},
	{Name: "RT_FILED_CFG", Value: RT_FILED_CFG, Hex: "0x0405", Comment: "配置文件升级/更新", SourceLine: 140},
	{Name: "RT_FILL_DATA", Value: RT_FILL_DATA, Hex: "0x0406", Comment: "补线处理", SourceLine: 141},
	{Name: "RT_TECHDATA_BYPERIOD", Value: RT_TECHDATA_BYPERIOD, Hex: "0x0407", Comment: "盘后分析扩展 -- 支持不同周期转换", SourceLine: 142},
	{Name: "RT_ONE_BLOCKDATA", Value: RT_ONE_BLOCKDATA, Hex: "0x0408", Comment: "一个板块数据请求", SourceLine: 143},
	{Name: "RT_TECHDATA_INCREMENT", Value: RT_TECHDATA_INCREMENT, Hex: "0x0409", Comment: "盘后分析扩展 -- 增量数据请求", SourceLine: 144},
	{Name: "RT_TECHDATA_RANGE", Value: RT_TECHDATA_RANGE, Hex: "0x0410", Comment: "分段盘后分析", SourceLine: 145},
	{Name: "RT_TECHDATA_BIGINT", Value: RT_TECHDATA_BIGINT, Hex: "0x0411", Comment: "盘后分析大数据月线以及年线", SourceLine: 146},
	{Name: "RT_TECHDATA_RANGE_BIGINT", Value: RT_TECHDATA_RANGE_BIGINT, Hex: "0x0412", Comment: "按时间区段请求K线 盘后分析大数据月线以及年线", SourceLine: 147},
	{Name: "RT_TECHDATA_AFTER", Value: RT_TECHDATA_AFTER, Hex: "0x0413", Comment: "科创板K线请求//????????", SourceLine: 291},
	{Name: "RT_TEXTDATAWITHINDEX_PLUS", Value: RT_TEXTDATAWITHINDEX_PLUS, Hex: "0x0501", Comment: "正序资讯索引数据", SourceLine: 150},
	{Name: "RT_TEXTDATAWITHINDEX_NEGATIVE", Value: RT_TEXTDATAWITHINDEX_NEGATIVE, Hex: "0x0502", Comment: "倒序资讯索引数据", SourceLine: 151},
	{Name: "RT_BYINDEXRETDATA", Value: RT_BYINDEXRETDATA, Hex: "0x0503", Comment: "资讯内容数据", SourceLine: 152},
	{Name: "RT_USERTEXTDATA", Value: RT_USERTEXTDATA, Hex: "0x0504", Comment: "自定义资讯请求（如菜单等）", SourceLine: 153},
	{Name: "RT_FILEREQUEST", Value: RT_FILEREQUEST, Hex: "0x0505", Comment: "配置文件文件", SourceLine: 154},
	{Name: "RT_FILESimplify", Value: RT_FILESimplify, Hex: "0x0506", Comment: "精简文件请求", SourceLine: 155},
	{Name: "RT_ATTATCHDATA", Value: RT_ATTATCHDATA, Hex: "0x0507", Comment: "附件数据", SourceLine: 156},
	{Name: "RT_PROMPT_INFO", Value: RT_PROMPT_INFO, Hex: "0x0508", Comment: "服务器设置的提示信息", SourceLine: 157},
	{Name: "RT_STOCKTICK", Value: RT_STOCKTICK, Hex: "0x0601", Comment: "个股分笔、个股详细的分笔数据", SourceLine: 159},
	{Name: "RT_BUYSELLORDER", Value: RT_BUYSELLORDER, Hex: "0x0602", Comment: "个股买卖盘", SourceLine: 160},
	{Name: "RT_LIMITTICK", Value: RT_LIMITTICK, Hex: "0x0603", Comment: "指定长度的分笔请求", SourceLine: 161},
	{Name: "RT_HISTORYTICK", Value: RT_HISTORYTICK, Hex: "0x0604", Comment: "历史的分笔请求", SourceLine: 162},
	{Name: "RT_MAJORINDEXTICK", Value: RT_MAJORINDEXTICK, Hex: "0x0605", Comment: "大盘明细", SourceLine: 163},
	{Name: "RT_VALUE", Value: RT_VALUE, Hex: "0x0606", Comment: "右小图“值”", SourceLine: 164},
	{Name: "RT_BUYSELLORDER_HK", Value: RT_BUYSELLORDER_HK, Hex: "0x0607", Comment: "个股买卖盘(港股）", SourceLine: 165},
	{Name: "RT_BUYSELLORDER_FUTURES", Value: RT_BUYSELLORDER_FUTURES, Hex: "0x0608", Comment: "个股买卖盘(期货）", SourceLine: 166},
	{Name: "RT_VALUE_HK", Value: RT_VALUE_HK, Hex: "0x0609", Comment: "右小图“值”(港股),右小图也发此请求", SourceLine: 167},
	{Name: "RT_VALUE_FUTURES", Value: RT_VALUE_FUTURES, Hex: "0x060A", Comment: "右小图“值”(期货),左下小图也发此请求", SourceLine: 169},
	{Name: "RT_TOTAL", Value: RT_TOTAL, Hex: "0x060B", Comment: "总持请求包", SourceLine: 170},
	{Name: "RT_LIMITTICK_EX", Value: RT_LIMITTICK_EX, Hex: "0x060C", Comment: "指定长度的分笔请求", SourceLine: 171},
	{Name: "RT_MINUTE_TICK", Value: RT_MINUTE_TICK, Hex: "0x0612", Comment: "/< 根据指定的分钟和偏移请", SourceLine: 168},
	{Name: "RT_AFTER_TICK", Value: RT_AFTER_TICK, Hex: "0x0617", Comment: "科创板盘后请求分笔//????????????", SourceLine: 290},
	{Name: "RT_LEAD", Value: RT_LEAD, Hex: "0x0702", Comment: "大盘领先指标", SourceLine: 174},
	{Name: "RT_MAJORINDEXTREND", Value: RT_MAJORINDEXTREND, Hex: "0x0703", Comment: "大盘走势", SourceLine: 175},
	{Name: "RT_MAJORINDEXADL", Value: RT_MAJORINDEXADL, Hex: "0x0704", Comment: "大盘走势－ADL", SourceLine: 176},
	{Name: "RT_MAJORINDEXDBBI", Value: RT_MAJORINDEXDBBI, Hex: "0x0705", Comment: "大盘走势－多空指标", SourceLine: 177},
	{Name: "RT_MAJORINDEXBUYSELL", Value: RT_MAJORINDEXBUYSELL, Hex: "0x0706", Comment: "大盘走势－买卖力道", SourceLine: 178},
	{Name: "RT_SERVERFILEINFO", Value: RT_SERVERFILEINFO, Hex: "0x0707", Comment: "服务器自动推送要更新的文件信息", SourceLine: 179},
	{Name: "RT_DOWNSERVERFILEINFO", Value: RT_DOWNSERVERFILEINFO, Hex: "0x0708", Comment: "下载-服务器自动推送要更新的文件信息", SourceLine: 180},
	{Name: "RT_LEVEL2_SZZB", Value: RT_LEVEL2_SZZB, Hex: "0x07D1", Comment: "2001", SourceLine: 454},
	{Name: "RT_LEVEL2_SZWTDL", Value: RT_LEVEL2_SZWTDL, Hex: "0x07D2", Comment: "2002，深证委托队列", SourceLine: 462},
	{Name: "RT_LEVEL2_SZKZ", Value: RT_LEVEL2_SZKZ, Hex: "0x07D4", Comment: "2004", SourceLine: 458},
	{Name: "RT_LEVEL2_SZKZ_PH", Value: RT_LEVEL2_SZKZ_PH, Hex: "0x07DB", Comment: "2011, 深圳盘后快照", SourceLine: 459},
	{Name: "RT_LEVEL2_SZKZ_TD", Value: RT_LEVEL2_SZKZ_TD, Hex: "0x07DE", Comment: "2014", SourceLine: 460},
	{Name: "RT_CURRENTFINANCEDATA", Value: RT_CURRENTFINANCEDATA, Hex: "0x0801", Comment: "最新的财务数据", SourceLine: 182},
	{Name: "RT_HISFINANCEDATA", Value: RT_HISFINANCEDATA, Hex: "0x0802", Comment: "历史财务数据", SourceLine: 183},
	{Name: "RT_EXRIGHT_DATA", Value: RT_EXRIGHT_DATA, Hex: "0x0803", Comment: "除权数据", SourceLine: 184},
	{Name: "RT_HK_RECORDOPTION", Value: RT_HK_RECORDOPTION, Hex: "0x0804", Comment: "港股期权", SourceLine: 185},
	{Name: "RT_BROKER_HK", Value: RT_BROKER_HK, Hex: "0x0805", Comment: "港股经纪席位下委托情况							// 看我们的服务器是否生成此数据", SourceLine: 186},
	{Name: "RT_BLOCK_DATA", Value: RT_BLOCK_DATA, Hex: "0x0806", Comment: "板块数据", SourceLine: 187},
	{Name: "RT_STATIC_HK", Value: RT_STATIC_HK, Hex: "0x0807", Comment: "港股静态数据", SourceLine: 188},
	{Name: "RT_USER_BLOCK_DATA", Value: RT_USER_BLOCK_DATA, Hex: "0x0810", Comment: "后台自定义板块数据							//add by lxqi 20090907", SourceLine: 189},
	{Name: "RT_HK_SH_BREAKER_VCM", Value: RT_HK_SH_BREAKER_VCM, Hex: "0x0812", Comment: "/< 港股市调机制信息请求", SourceLine: 190},
	{Name: "RT_HK_SH_BREAKER_CAS", Value: RT_HK_SH_BREAKER_CAS, Hex: "0x0814", Comment: "/< 港股收市竞价信息请求", SourceLine: 191},
	{Name: "RT_MASSDATA", Value: RT_MASSDATA, Hex: "0x0901", Comment: "大单", SourceLine: 193},
	{Name: "RT_SERVERTIME", Value: RT_SERVERTIME, Hex: "0x0902", Comment: "服务器当前时间", SourceLine: 194},
	{Name: "RT_KEEPACTIVE", Value: RT_KEEPACTIVE, Hex: "0x0903", Comment: "保活通信包", SourceLine: 195},
	{Name: "RT_TEST", Value: RT_TEST, Hex: "0x0904", Comment: "测试通信包", SourceLine: 196},
	{Name: "RT_TESTSRV", Value: RT_TESTSRV, Hex: "0x0905", Comment: "测试客户端到服务器是否通畅", SourceLine: 197},
	{Name: "RT_PUSHINFODATA", Value: RT_PUSHINFODATA, Hex: "0x0906", Comment: "资讯实时主推", SourceLine: 198},
	{Name: "RT_MARKET_MONITOR_SUPPORT_QUERY", Value: RT_MARKET_MONITOR_SUPPORT_QUERY, Hex: "0x0907", Comment: "短线精灵订阅功能号", SourceLine: 201},
	{Name: "RT_MARKET_MONITOR_STOCK_QUERY", Value: RT_MARKET_MONITOR_STOCK_QUERY, Hex: "0x0908", Comment: "短线精灵按股票查询", SourceLine: 202},
	{Name: "RT_MARKET_MONITOR_DATE_QUERY", Value: RT_MARKET_MONITOR_DATE_QUERY, Hex: "0x0909", Comment: "短线精灵按日期查询", SourceLine: 203},
	{Name: "RT_AUTOPUSH", Value: RT_AUTOPUSH, Hex: "0x0A01", Comment: "常用主推									// 改RealTimeData 为 CommRealTimeData", SourceLine: 205},
	{Name: "RT_AUTOPUSHSIMP", Value: RT_AUTOPUSHSIMP, Hex: "0x0A02", Comment: "精简主推									// 改为请求主推", SourceLine: 206},
	{Name: "RT_REQAUTOPUSH", Value: RT_REQAUTOPUSH, Hex: "0x0A03", Comment: "请求主推,应用于：预警、跑马灯			// 改RealTimeData 为 CommRealTimeData", SourceLine: 207},
	{Name: "RT_ETF_AUTOPUSH", Value: RT_ETF_AUTOPUSH, Hex: "0x0A04", Comment: "ETF主推", SourceLine: 208},
	{Name: "RT_AUTOBROKER_HK", Value: RT_AUTOBROKER_HK, Hex: "0x0A05", Comment: "经纪主推", SourceLine: 209},
	{Name: "RT_AUTOTICK_HK", Value: RT_AUTOTICK_HK, Hex: "0x0A06", Comment: "港股分笔主推", SourceLine: 210},
	{Name: "RT_AUTOPUSH_QH", Value: RT_AUTOPUSH_QH, Hex: "0x0A07", Comment: "期货最小主推", SourceLine: 211},
	{Name: "RT_PUSHREALTIMEINFO", Value: RT_PUSHREALTIMEINFO, Hex: "0x0A08", Comment: "实时解盘主推", SourceLine: 212},
	{Name: "RT_RAW_AUTOPUSH", Value: RT_RAW_AUTOPUSH, Hex: "0x0A09", Comment: "数据源原始数据主推", SourceLine: 213},
	{Name: "RT_QHMM_AUTOPUSH", Value: RT_QHMM_AUTOPUSH, Hex: "0x0A0A", Comment: "期货买卖盘主推", SourceLine: 216},
	{Name: "RT_LEVEL_AUTOPUSH", Value: RT_LEVEL_AUTOPUSH, Hex: "0x0A0B", Comment: "level主推", SourceLine: 217},
	{Name: "RT_LEVEL_BORDERQUEUE_AUTOPUSH", Value: RT_LEVEL_BORDERQUEUE_AUTOPUSH, Hex: "0x0A0C", Comment: "LEVEL2买卖队列买方向主推", SourceLine: 234},
	{Name: "RT_LEVEL_SORDERQUEUE_AUTOPUSH", Value: RT_LEVEL_SORDERQUEUE_AUTOPUSH, Hex: "0x0A0D", Comment: "LEVEL2买卖队列卖方向主推", SourceLine: 235},
	{Name: "RT_LEVEL_TRANSACTION_AUTOPUSH", Value: RT_LEVEL_TRANSACTION_AUTOPUSH, Hex: "0x0A0E", Comment: "LEVEL2逐笔成交主推", SourceLine: 236},
	{Name: "RT_AUTOPUSH_EXT", Value: RT_AUTOPUSH_EXT, Hex: "0x0A0F", Comment: "", SourceLine: 229},
	{Name: "RT_AUTO_TOTALMAX", Value: RT_AUTO_TOTALMAX, Hex: "0x0A10", Comment: "", SourceLine: 224},
	{Name: "RT_AUTO_SINGLEMAX", Value: RT_AUTO_SINGLEMAX, Hex: "0x0A11", Comment: "", SourceLine: 225},
	{Name: "RT_AUTOPUSH_RES", Value: RT_AUTOPUSH_RES, Hex: "0x0A12", Comment: "预留的常用主推 maxy add", SourceLine: 214},
	{Name: "RT_AUTOPUSH_LV2RES", Value: RT_AUTOPUSH_LV2RES, Hex: "0x0A13", Comment: "预留的level2主推", SourceLine: 215},
	{Name: "RT_MARKET_MONITOR_AUTOPUSH", Value: RT_MARKET_MONITOR_AUTOPUSH, Hex: "0x0A17", Comment: "短线精灵主推功能号", SourceLine: 218},
	{Name: "RT_DYNAUCTION_AUTOPUSH", Value: RT_DYNAUCTION_AUTOPUSH, Hex: "0x0A18", Comment: "文交所动态竞价主推", SourceLine: 219},
	{Name: "RT_FIXPRICE_AUTOPUSH", Value: RT_FIXPRICE_AUTOPUSH, Hex: "0x0A1A", Comment: "武汉光谷定价交易主推", SourceLine: 220},
	{Name: "RT_HK_SH_BREAKER_VCM_AUTOPUSH", Value: RT_HK_SH_BREAKER_VCM_AUTOPUSH, Hex: "0x0A29", Comment: "/< 港股市调机制主推订阅", SourceLine: 221},
	{Name: "RT_HK_SH_BREAKER_CAS_AUTOPUSH", Value: RT_HK_SH_BREAKER_CAS_AUTOPUSH, Hex: "0x0A30", Comment: "/< 收市竞价主推订阅", SourceLine: 222},
	{Name: "RT_AUTOPUSH_SENIOR", Value: RT_AUTOPUSH_SENIOR, Hex: "0x0A32", Comment: "行情多档快照主推数据功能号", SourceLine: 119},
	{Name: "RT_UPDATEDFINANCIALDATA", Value: RT_UPDATEDFINANCIALDATA, Hex: "0x0B01", Comment: "增量的财务报表数据", SourceLine: 239},
	{Name: "RT_SYNCHRONIZATIONDATA", Value: RT_SYNCHRONIZATIONDATA, Hex: "0x0B02", Comment: "数据同步处理", SourceLine: 240},
	{Name: "RT_MARKET_NOTICE", Value: RT_MARKET_NOTICE, Hex: "0x0B03", Comment: "主推市场信息（不向服务器订阅就可收到服务器主推消息）", SourceLine: 241},
	{Name: "RT_Send_Notice", Value: RT_Send_Notice, Hex: "0x0C01", Comment: "发表公告", SourceLine: 244},
	{Name: "RT_Send_ScrollText", Value: RT_Send_ScrollText, Hex: "0x0C02", Comment: "发表滚动信息", SourceLine: 245},
	{Name: "RT_Change_program", Value: RT_Change_program, Hex: "0x0C03", Comment: "更改服务器程序", SourceLine: 246},
	{Name: "RT_Send_File_Data", Value: RT_Send_File_Data, Hex: "0x0C04", Comment: "发送文件到服务器", SourceLine: 247},
	{Name: "RT_RequestDBF", Value: RT_RequestDBF, Hex: "0x0C05", Comment: "请求DBF文件", SourceLine: 248},
	{Name: "RT_InfoSend", Value: RT_InfoSend, Hex: "0x0C06", Comment: "发布信息", SourceLine: 250},
	{Name: "RT_InfoUpdateIndex", Value: RT_InfoUpdateIndex, Hex: "0x0C07", Comment: "更新信息索引", SourceLine: 251},
	{Name: "RT_InfoUpdateOneIndex", Value: RT_InfoUpdateOneIndex, Hex: "0x0C08", Comment: "更新一条信息索引", SourceLine: 252},
	{Name: "RT_NoteMsgData", Value: RT_NoteMsgData, Hex: "0x0C09", Comment: "定制短信数据传送", SourceLine: 253},
	{Name: "RT_InfoDataTransmit", Value: RT_InfoDataTransmit, Hex: "0x0C0A", Comment: "验证转发", SourceLine: 254},
	{Name: "RT_InfoCheckPurview", Value: RT_InfoCheckPurview, Hex: "0x0C0B", Comment: "返回注册详细分类信息", SourceLine: 255},
	{Name: "RT_InfoClickTime", Value: RT_InfoClickTime, Hex: "0x0C0C", Comment: "点击次数", SourceLine: 256},
	{Name: "RT_REPORTSORT_Simple", Value: RT_REPORTSORT_Simple, Hex: "0x0D01", Comment: "排名报价表:61-66、点击列排序（精简）", SourceLine: 258},
	{Name: "RT_PARTINITIALINFO_Simple", Value: RT_PARTINITIALINFO_Simple, Hex: "0x0D02", Comment: "代码返回", SourceLine: 259},
	{Name: "RT_RETURN_EMPTY", Value: RT_RETURN_EMPTY, Hex: "0x0D03", Comment: "返回空的数据包", SourceLine: 260},
	{Name: "RT_InfoDataRailing", Value: RT_InfoDataRailing, Hex: "0x0D04", Comment: "请求栏目", SourceLine: 261},
	{Name: "RT_Srv_SrvStatus", Value: RT_Srv_SrvStatus, Hex: "0x0F01", Comment: "后台程序运行状态", SourceLine: 263},
	{Name: "RT_SRV_SYNC", Value: RT_SRV_SYNC, Hex: "0x0F02", Comment: "客户端和服务器维护一个Session同步", SourceLine: 264},
	{Name: "RT_Srv_Sub_Succ", Value: RT_Srv_Sub_Succ, Hex: "0x1000", Comment: "操作成功提示", SourceLine: 336},
	{Name: "RT_REPORTSORT_INT64", Value: RT_REPORTSORT_INT64, Hex: "0x1203", Comment: "报价排名", SourceLine: 268},
	{Name: "RT_GENERALSORT_EX_INT64", Value: RT_GENERALSORT_EX_INT64, Hex: "0x1205", Comment: "综合排名报表", SourceLine: 269},
	{Name: "RT_LEVEL_REALTIME_INT64", Value: RT_LEVEL_REALTIME_INT64, Hex: "0x120A", Comment: "Level2实时行情", SourceLine: 271},
	{Name: "RT_REALTIME_EXT_INT64", Value: RT_REALTIME_EXT_INT64, Hex: "0x120F", Comment: "扩展行情报价表", SourceLine: 267},
	{Name: "RT_MARKETSORT_INT64", Value: RT_MARKETSORT_INT64, Hex: "0x1217", Comment: "按指定的行情列对市场排序", SourceLine: 270},
	{Name: "RT_TREND_INT64", Value: RT_TREND_INT64, Hex: "0x1301", Comment: "分时走势（待定）", SourceLine: 272},
	{Name: "RT_ADDTREND_INT64", Value: RT_ADDTREND_INT64, Hex: "0x1302", Comment: "走势图叠加、多股同列", SourceLine: 273},
	{Name: "RT_BUYSELLPOWER_INT64", Value: RT_BUYSELLPOWER_INT64, Hex: "0x1303", Comment: "买卖力道", SourceLine: 275},
	{Name: "RT_HISTREND_INT64", Value: RT_HISTREND_INT64, Hex: "0x1304", Comment: "历史回忆、多日分时、右小图下分时走势", SourceLine: 276},
	{Name: "RT_TICK_INT64", Value: RT_TICK_INT64, Hex: "0x1305", Comment: "tick图（待定）", SourceLine: 278},
	{Name: "RT_HISTREND_INDEX_INT64", Value: RT_HISTREND_INDEX_INT64, Hex: "0x1309", Comment: "对于大盘领先-历史回忆;多日分时;右小图下分时走势（待定）", SourceLine: 277},
	{Name: "RT_TREND_EXT_INT64", Value: RT_TREND_EXT_INT64, Hex: "0x130B", Comment: "扩展分时走势", SourceLine: 274},
	{Name: "RT_STOCKTICK_INT64", Value: RT_STOCKTICK_INT64, Hex: "0x1601", Comment: "个股分笔", SourceLine: 280},
	{Name: "RT_LIMITTICK_INT64", Value: RT_LIMITTICK_INT64, Hex: "0x1603", Comment: "指定长度的分笔请求", SourceLine: 281},
	{Name: "RT_MAJORINDEXTICK_INT64", Value: RT_MAJORINDEXTICK_INT64, Hex: "0x1605", Comment: "大盘明细（待定）", SourceLine: 279},
	{Name: "RT_VALUE_INT64", Value: RT_VALUE_INT64, Hex: "0x1606", Comment: "数值", SourceLine: 284},
	{Name: "RT_STOCKTICK_SEARCH_INT64", Value: RT_STOCKTICK_SEARCH_INT64, Hex: "0x160C", Comment: "分笔查询（待定）", SourceLine: 282},
	{Name: "RT_MINUTE_TICK_INT64", Value: RT_MINUTE_TICK_INT64, Hex: "0x1612", Comment: "根据指定的分钟和偏移请求指定数量的分笔", SourceLine: 283},
	{Name: "RT_LEAD_INT64", Value: RT_LEAD_INT64, Hex: "0x1702", Comment: "/< 大盘领先指标", SourceLine: 285},
	{Name: "RT_MAJORINDEXTREND_INT64", Value: RT_MAJORINDEXTREND_INT64, Hex: "0x1703", Comment: "大盘走势", SourceLine: 286},
	{Name: "RT_LEVEL_AUTOPUSH_INT64", Value: RT_LEVEL_AUTOPUSH_INT64, Hex: "0x1A0B", Comment: "Level2主推", SourceLine: 288},
	{Name: "RT_AUTOPUSH_EXT_INT64", Value: RT_AUTOPUSH_EXT_INT64, Hex: "0x1A0F", Comment: "主推", SourceLine: 287},
	{Name: "RT_LEVEL_TOTALMAX", Value: RT_LEVEL_TOTALMAX, Hex: "0x2001", Comment: "", SourceLine: 226},
	{Name: "RT_LEVEL_SINGLEMAX", Value: RT_LEVEL_SINGLEMAX, Hex: "0x2002", Comment: "", SourceLine: 227},
	{Name: "RT_ZIPDATA", Value: RT_ZIPDATA, Hex: "0x8001", Comment: "压缩返回包类型", SourceLine: 52},
	{Name: "RT_COMPASKDATA", Value: RT_COMPASKDATA, Hex: "0x8FFE", Comment: "组合请求类型", SourceLine: 51},
	{Name: "RT_END", Value: RT_END, Hex: "0x8FFF", Comment: "", SourceLine: 49},
	{Name: "RT_LEVEL2_SHWTDL_TD", Value: RT_LEVEL2_SHWTDL_TD, Hex: "0xF3EA", Comment: "", SourceLine: 452},
	{Name: "RT_LEVEL2_SHKZ_TD", Value: RT_LEVEL2_SHKZ_TD, Hex: "0xF3EC", Comment: "", SourceLine: 448},
	{Name: "RT_LEVEL2_SHZB_TD", Value: RT_LEVEL2_SHZB_TD, Hex: "0xF3F9", Comment: "", SourceLine: 444},
	{Name: "RT_LEVEL2_SZZB_TD", Value: RT_LEVEL2_SZZB_TD, Hex: "0xF7D1", Comment: "", SourceLine: 455},
	{Name: "RT_LEVEL2_SZWTDL_TD", Value: RT_LEVEL2_SZWTDL_TD, Hex: "0xF7D2", Comment: "", SourceLine: 463},
}
