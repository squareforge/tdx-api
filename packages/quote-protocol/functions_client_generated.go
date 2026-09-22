// Code generated from functions_generated.go; DO NOT EDIT.
package quoteprotocol

import (
	"context"
	"time"
)

type FunctionCallOptions struct {
	ExpectType uint16
	Size       int16
	Option     int16
	Payload    []byte
	Timeout    time.Duration
}

func (c *Client) callFunction(ctx context.Context, functionType uint16, opts FunctionCallOptions) (*Response, error) {
	expectType := opts.ExpectType
	if expectType == 0 {
		expectType = functionType
	}
	return c.Request(ctx, Request{
		Type:       functionType,
		ExpectType: expectType,
		Size:       opts.Size,
		Option:     opts.Option,
		Payload:    opts.Payload,
		Timeout:    opts.Timeout,
	})
}

func (c *Client) CallRT_RISE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_RISE, opts)
}

func (c *Client) CallRT_Srv_Sub_Restart(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_Restart, opts)
}

func (c *Client) CallRT_FALL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FALL, opts)
}

func (c *Client) CallRT_Srv_Sub_Replace(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_Replace, opts)
}

func (c *Client) CallRT_Srv_Sub_DownCFG(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_DownCFG, opts)
}

func (c *Client) CallRT_5_RISE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_5_RISE, opts)
}

func (c *Client) CallRT_Srv_Sub_UpCFG(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_UpCFG, opts)
}

func (c *Client) CallRT_Srv_Sub_DownUserDB(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_DownUserDB, opts)
}

func (c *Client) CallRT_Srv_Sub_UpUserDB(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_UpUserDB, opts)
}

func (c *Client) CallRT_Srv_Sub_DownReport(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_DownReport, opts)
}

func (c *Client) CallRT_5_FALL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_5_FALL, opts)
}

func (c *Client) CallRT_Srv_Sub_LimitPrompt(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_LimitPrompt, opts)
}

func (c *Client) CallRT_AHEAD_COMM(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AHEAD_COMM, opts)
}

func (c *Client) CallRT_JAVA_MARK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_JAVA_MARK, opts)
}

func (c *Client) CallRT_AFTER_COMM(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AFTER_COMM, opts)
}

func (c *Client) CallRT_WINCE_MARK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_WINCE_MARK, opts)
}

func (c *Client) CallRT_AHEAD_PRICE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AHEAD_PRICE, opts)
}

func (c *Client) CallRT_NOTZIP(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_NOTZIP, opts)
}

func (c *Client) CallRT_AHEAD_VOLBI(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AHEAD_VOLBI, opts)
}

func (c *Client) CallRT_DEBUG(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DEBUG, opts)
}

func (c *Client) CallRT_AHEAD_MONEY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AHEAD_MONEY, opts)
}

func (c *Client) CallRT_BEGIN(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BEGIN, opts)
}

func (c *Client) CallRT_INITIALINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_INITIALINFO, opts)
}

func (c *Client) CallRT_LOGIN(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN, opts)
}

func (c *Client) CallRT_SERVERINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SERVERINFO, opts)
}

func (c *Client) CallRT_BULLETIN(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BULLETIN, opts)
}

func (c *Client) CallRT_PARTINITIALINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_PARTINITIALINFO, opts)
}

func (c *Client) CallRT_ETF_INIT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ETF_INIT, opts)
}

func (c *Client) CallRT_LOGIN_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_HK, opts)
}

func (c *Client) CallRT_LOGIN_FUTURES(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_FUTURES, opts)
}

func (c *Client) CallRT_LOGIN_FOREIGN(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_FOREIGN, opts)
}

func (c *Client) CallRT_LOGIN_WP(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_WP, opts)
}

func (c *Client) CallRT_LOGIN_INFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_INFO, opts)
}

func (c *Client) CallRT_CHANGE_PWD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_CHANGE_PWD, opts)
}

func (c *Client) CallRT_LOGIN_LEVEL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_LEVEL, opts)
}

func (c *Client) CallRT_SERVERINFO2(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SERVERINFO2, opts)
}

func (c *Client) CallRT_VERIFYINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_VERIFYINFO, opts)
}

func (c *Client) CallRT_LOGIN_CENTER(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_CENTER, opts)
}

func (c *Client) CallRT_LOGIN_FTP(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_FTP, opts)
}

func (c *Client) CallRT_MODIFY_LV2PWD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MODIFY_LV2PWD, opts)
}

func (c *Client) CallRT_MODIFY_PWD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MODIFY_PWD, opts)
}

func (c *Client) CallRT_DISCONNLEVEL2(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DISCONNLEVEL2, opts)
}

func (c *Client) CallRT_STOCKINFOLIST(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_STOCKINFOLIST, opts)
}

func (c *Client) CallRT_LOGIN_COMMODITIES(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_COMMODITIES, opts)
}

func (c *Client) CallRT_LOGIN_CAE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_CAE, opts)
}

func (c *Client) CallRT_LOGIN_US(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_US, opts)
}

func (c *Client) CallRT_LOGIN_OPTION(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LOGIN_OPTION, opts)
}

func (c *Client) CallRT_INITIALINFO_VARCODE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_INITIALINFO_VARCODE, opts)
}

func (c *Client) CallRT_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REALTIME, opts)
}

func (c *Client) CallRT_DYNREPORT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DYNREPORT, opts)
}

func (c *Client) CallRT_REPORTSORT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REPORTSORT, opts)
}

func (c *Client) CallRT_GENERALSORT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_GENERALSORT, opts)
}

func (c *Client) CallRT_GENERALSORT_EX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_GENERALSORT_EX, opts)
}

func (c *Client) CallRT_SEVER_EMPTY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SEVER_EMPTY, opts)
}

func (c *Client) CallRT_SEVER_CALCULATE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SEVER_CALCULATE, opts)
}

func (c *Client) CallRT_ANS_BYTYPE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ANS_BYTYPE, opts)
}

func (c *Client) CallRT_QHMM_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_QHMM_REALTIME, opts)
}

func (c *Client) CallRT_LEVEL_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_REALTIME, opts)
}

func (c *Client) CallRT_CLASS_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_CLASS_REALTIME, opts)
}

func (c *Client) CallRT_LEVEL_ORDERQUEUE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_ORDERQUEUE, opts)
}

func (c *Client) CallRT_LEVEL_TRANSACTION(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_TRANSACTION, opts)
}

func (c *Client) CallRT_QHARB_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_QHARB_REALTIME, opts)
}

func (c *Client) CallRT_REALTIME_EXT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REALTIME_EXT, opts)
}

func (c *Client) CallRT_REPORTSORT_EXT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REPORTSORT_EXT, opts)
}

func (c *Client) CallRT_HQCOL_VALUE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HQCOL_VALUE, opts)
}

func (c *Client) CallRT_MARKETSORT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKETSORT, opts)
}

func (c *Client) CallRT_ASSIST_QUERY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ASSIST_QUERY, opts)
}

func (c *Client) CallRT_DYNAUCTION_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DYNAUCTION_REALTIME, opts)
}

func (c *Client) CallRT_DYNAUCTION_TICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DYNAUCTION_TICK, opts)
}

func (c *Client) CallRT_GET_TABLE_ALLDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_GET_TABLE_ALLDATA, opts)
}

func (c *Client) CallRT_FIXED_PRICE_REALTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FIXED_PRICE_REALTIME, opts)
}

func (c *Client) CallRT_GET_RELAVENT_BLOCK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_GET_RELAVENT_BLOCK, opts)
}

func (c *Client) CallRT_REALTIME_SENIOR(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REALTIME_SENIOR, opts)
}

func (c *Client) CallRT_TREND(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TREND, opts)
}

func (c *Client) CallRT_ADDTREND(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ADDTREND, opts)
}

func (c *Client) CallRT_BUYSELLPOWER(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BUYSELLPOWER, opts)
}

func (c *Client) CallRT_HISTREND(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISTREND, opts)
}

func (c *Client) CallRT_TICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TICK, opts)
}

func (c *Client) CallRT_ETF_TREND(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ETF_TREND, opts)
}

func (c *Client) CallRT_ETF_NOWDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ETF_NOWDATA, opts)
}

func (c *Client) CallRT_ETF_TREND_TECH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ETF_TREND_TECH, opts)
}

func (c *Client) CallRT_HISTREND_INDEX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISTREND_INDEX, opts)
}

func (c *Client) CallRT_AUTOARBPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOARBPUSH, opts)
}

func (c *Client) CallRT_TREND_EXT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TREND_EXT, opts)
}

func (c *Client) CallRT_HISTREND_EXT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISTREND_EXT, opts)
}

func (c *Client) CallRT_VIRTUAL_AUCTION(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_VIRTUAL_AUCTION, opts)
}

func (c *Client) CallRT_LEVEL2_SHZB(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHZB, opts)
}

func (c *Client) CallRT_LEVEL2_SHWTDL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHWTDL, opts)
}

func (c *Client) CallRT_LEVEL2_SHKZ(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHKZ, opts)
}

func (c *Client) CallRT_LEVEL2_SHKZ_PH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHKZ_PH, opts)
}

func (c *Client) CallRT_TECHDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA, opts)
}

func (c *Client) CallRT_FILEDOWNLOAD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FILEDOWNLOAD, opts)
}

func (c *Client) CallRT_TECHDATA_EX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_EX, opts)
}

func (c *Client) CallRT_DATA_WEIHU(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DATA_WEIHU, opts)
}

func (c *Client) CallRT_FILEDOWNLOAD2(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FILEDOWNLOAD2, opts)
}

func (c *Client) CallRT_FILED_CFG(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FILED_CFG, opts)
}

func (c *Client) CallRT_FILL_DATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FILL_DATA, opts)
}

func (c *Client) CallRT_TECHDATA_BYPERIOD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_BYPERIOD, opts)
}

func (c *Client) CallRT_ONE_BLOCKDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ONE_BLOCKDATA, opts)
}

func (c *Client) CallRT_TECHDATA_INCREMENT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_INCREMENT, opts)
}

func (c *Client) CallRT_TECHDATA_RANGE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_RANGE, opts)
}

func (c *Client) CallRT_TECHDATA_BIGINT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_BIGINT, opts)
}

func (c *Client) CallRT_TECHDATA_RANGE_BIGINT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_RANGE_BIGINT, opts)
}

func (c *Client) CallRT_TECHDATA_AFTER(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TECHDATA_AFTER, opts)
}

func (c *Client) CallRT_TEXTDATAWITHINDEX_PLUS(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TEXTDATAWITHINDEX_PLUS, opts)
}

func (c *Client) CallRT_TEXTDATAWITHINDEX_NEGATIVE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TEXTDATAWITHINDEX_NEGATIVE, opts)
}

func (c *Client) CallRT_BYINDEXRETDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BYINDEXRETDATA, opts)
}

func (c *Client) CallRT_USERTEXTDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_USERTEXTDATA, opts)
}

func (c *Client) CallRT_FILEREQUEST(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FILEREQUEST, opts)
}

func (c *Client) CallRT_FILESimplify(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FILESimplify, opts)
}

func (c *Client) CallRT_ATTATCHDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ATTATCHDATA, opts)
}

func (c *Client) CallRT_PROMPT_INFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_PROMPT_INFO, opts)
}

func (c *Client) CallRT_STOCKTICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_STOCKTICK, opts)
}

func (c *Client) CallRT_BUYSELLORDER(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BUYSELLORDER, opts)
}

func (c *Client) CallRT_LIMITTICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LIMITTICK, opts)
}

func (c *Client) CallRT_HISTORYTICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISTORYTICK, opts)
}

func (c *Client) CallRT_MAJORINDEXTICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXTICK, opts)
}

func (c *Client) CallRT_VALUE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_VALUE, opts)
}

func (c *Client) CallRT_BUYSELLORDER_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BUYSELLORDER_HK, opts)
}

func (c *Client) CallRT_BUYSELLORDER_FUTURES(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BUYSELLORDER_FUTURES, opts)
}

func (c *Client) CallRT_VALUE_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_VALUE_HK, opts)
}

func (c *Client) CallRT_VALUE_FUTURES(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_VALUE_FUTURES, opts)
}

func (c *Client) CallRT_TOTAL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TOTAL, opts)
}

func (c *Client) CallRT_LIMITTICK_EX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LIMITTICK_EX, opts)
}

func (c *Client) CallRT_MINUTE_TICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MINUTE_TICK, opts)
}

func (c *Client) CallRT_AFTER_TICK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AFTER_TICK, opts)
}

func (c *Client) CallRT_LEAD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEAD, opts)
}

func (c *Client) CallRT_MAJORINDEXTREND(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXTREND, opts)
}

func (c *Client) CallRT_MAJORINDEXADL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXADL, opts)
}

func (c *Client) CallRT_MAJORINDEXDBBI(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXDBBI, opts)
}

func (c *Client) CallRT_MAJORINDEXBUYSELL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXBUYSELL, opts)
}

func (c *Client) CallRT_SERVERFILEINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SERVERFILEINFO, opts)
}

func (c *Client) CallRT_DOWNSERVERFILEINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DOWNSERVERFILEINFO, opts)
}

func (c *Client) CallRT_LEVEL2_SZZB(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZZB, opts)
}

func (c *Client) CallRT_LEVEL2_SZWTDL(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZWTDL, opts)
}

func (c *Client) CallRT_LEVEL2_SZKZ(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZKZ, opts)
}

func (c *Client) CallRT_LEVEL2_SZKZ_PH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZKZ_PH, opts)
}

func (c *Client) CallRT_LEVEL2_SZKZ_TD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZKZ_TD, opts)
}

func (c *Client) CallRT_CURRENTFINANCEDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_CURRENTFINANCEDATA, opts)
}

func (c *Client) CallRT_HISFINANCEDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISFINANCEDATA, opts)
}

func (c *Client) CallRT_EXRIGHT_DATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_EXRIGHT_DATA, opts)
}

func (c *Client) CallRT_HK_RECORDOPTION(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HK_RECORDOPTION, opts)
}

func (c *Client) CallRT_BROKER_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BROKER_HK, opts)
}

func (c *Client) CallRT_BLOCK_DATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BLOCK_DATA, opts)
}

func (c *Client) CallRT_STATIC_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_STATIC_HK, opts)
}

func (c *Client) CallRT_USER_BLOCK_DATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_USER_BLOCK_DATA, opts)
}

func (c *Client) CallRT_HK_SH_BREAKER_VCM(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HK_SH_BREAKER_VCM, opts)
}

func (c *Client) CallRT_HK_SH_BREAKER_CAS(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HK_SH_BREAKER_CAS, opts)
}

func (c *Client) CallRT_MASSDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MASSDATA, opts)
}

func (c *Client) CallRT_SERVERTIME(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SERVERTIME, opts)
}

func (c *Client) CallRT_KEEPACTIVE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_KEEPACTIVE, opts)
}

func (c *Client) CallRT_TEST(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TEST, opts)
}

func (c *Client) CallRT_TESTSRV(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TESTSRV, opts)
}

func (c *Client) CallRT_PUSHINFODATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_PUSHINFODATA, opts)
}

func (c *Client) CallRT_MARKET_MONITOR_SUPPORT_QUERY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKET_MONITOR_SUPPORT_QUERY, opts)
}

func (c *Client) CallRT_MARKET_MONITOR_STOCK_QUERY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKET_MONITOR_STOCK_QUERY, opts)
}

func (c *Client) CallRT_MARKET_MONITOR_DATE_QUERY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKET_MONITOR_DATE_QUERY, opts)
}

func (c *Client) CallRT_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH, opts)
}

func (c *Client) CallRT_AUTOPUSHSIMP(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSHSIMP, opts)
}

func (c *Client) CallRT_REQAUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REQAUTOPUSH, opts)
}

func (c *Client) CallRT_ETF_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ETF_AUTOPUSH, opts)
}

func (c *Client) CallRT_AUTOBROKER_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOBROKER_HK, opts)
}

func (c *Client) CallRT_AUTOTICK_HK(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOTICK_HK, opts)
}

func (c *Client) CallRT_AUTOPUSH_QH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH_QH, opts)
}

func (c *Client) CallRT_PUSHREALTIMEINFO(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_PUSHREALTIMEINFO, opts)
}

func (c *Client) CallRT_RAW_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_RAW_AUTOPUSH, opts)
}

func (c *Client) CallRT_QHMM_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_QHMM_AUTOPUSH, opts)
}

func (c *Client) CallRT_LEVEL_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_AUTOPUSH, opts)
}

func (c *Client) CallRT_LEVEL_BORDERQUEUE_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_BORDERQUEUE_AUTOPUSH, opts)
}

func (c *Client) CallRT_LEVEL_SORDERQUEUE_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_SORDERQUEUE_AUTOPUSH, opts)
}

func (c *Client) CallRT_LEVEL_TRANSACTION_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_TRANSACTION_AUTOPUSH, opts)
}

func (c *Client) CallRT_AUTOPUSH_EXT(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH_EXT, opts)
}

func (c *Client) CallRT_AUTO_TOTALMAX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTO_TOTALMAX, opts)
}

func (c *Client) CallRT_AUTO_SINGLEMAX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTO_SINGLEMAX, opts)
}

func (c *Client) CallRT_AUTOPUSH_RES(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH_RES, opts)
}

func (c *Client) CallRT_AUTOPUSH_LV2RES(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH_LV2RES, opts)
}

func (c *Client) CallRT_MARKET_MONITOR_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKET_MONITOR_AUTOPUSH, opts)
}

func (c *Client) CallRT_DYNAUCTION_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_DYNAUCTION_AUTOPUSH, opts)
}

func (c *Client) CallRT_FIXPRICE_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_FIXPRICE_AUTOPUSH, opts)
}

func (c *Client) CallRT_HK_SH_BREAKER_VCM_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HK_SH_BREAKER_VCM_AUTOPUSH, opts)
}

func (c *Client) CallRT_HK_SH_BREAKER_CAS_AUTOPUSH(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HK_SH_BREAKER_CAS_AUTOPUSH, opts)
}

func (c *Client) CallRT_AUTOPUSH_SENIOR(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH_SENIOR, opts)
}

func (c *Client) CallRT_UPDATEDFINANCIALDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_UPDATEDFINANCIALDATA, opts)
}

func (c *Client) CallRT_SYNCHRONIZATIONDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SYNCHRONIZATIONDATA, opts)
}

func (c *Client) CallRT_MARKET_NOTICE(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKET_NOTICE, opts)
}

func (c *Client) CallRT_Send_Notice(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Send_Notice, opts)
}

func (c *Client) CallRT_Send_ScrollText(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Send_ScrollText, opts)
}

func (c *Client) CallRT_Change_program(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Change_program, opts)
}

func (c *Client) CallRT_Send_File_Data(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Send_File_Data, opts)
}

func (c *Client) CallRT_RequestDBF(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_RequestDBF, opts)
}

func (c *Client) CallRT_InfoSend(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoSend, opts)
}

func (c *Client) CallRT_InfoUpdateIndex(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoUpdateIndex, opts)
}

func (c *Client) CallRT_InfoUpdateOneIndex(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoUpdateOneIndex, opts)
}

func (c *Client) CallRT_NoteMsgData(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_NoteMsgData, opts)
}

func (c *Client) CallRT_InfoDataTransmit(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoDataTransmit, opts)
}

func (c *Client) CallRT_InfoCheckPurview(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoCheckPurview, opts)
}

func (c *Client) CallRT_InfoClickTime(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoClickTime, opts)
}

func (c *Client) CallRT_REPORTSORT_Simple(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REPORTSORT_Simple, opts)
}

func (c *Client) CallRT_PARTINITIALINFO_Simple(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_PARTINITIALINFO_Simple, opts)
}

func (c *Client) CallRT_RETURN_EMPTY(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_RETURN_EMPTY, opts)
}

func (c *Client) CallRT_InfoDataRailing(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_InfoDataRailing, opts)
}

func (c *Client) CallRT_Srv_SrvStatus(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_SrvStatus, opts)
}

func (c *Client) CallRT_SRV_SYNC(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_SRV_SYNC, opts)
}

func (c *Client) CallRT_Srv_Sub_Succ(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_Srv_Sub_Succ, opts)
}

func (c *Client) CallRT_REPORTSORT_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REPORTSORT_INT64, opts)
}

func (c *Client) CallRT_GENERALSORT_EX_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_GENERALSORT_EX_INT64, opts)
}

func (c *Client) CallRT_LEVEL_REALTIME_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_REALTIME_INT64, opts)
}

func (c *Client) CallRT_REALTIME_EXT_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_REALTIME_EXT_INT64, opts)
}

func (c *Client) CallRT_MARKETSORT_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MARKETSORT_INT64, opts)
}

func (c *Client) CallRT_TREND_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TREND_INT64, opts)
}

func (c *Client) CallRT_ADDTREND_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ADDTREND_INT64, opts)
}

func (c *Client) CallRT_BUYSELLPOWER_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_BUYSELLPOWER_INT64, opts)
}

func (c *Client) CallRT_HISTREND_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISTREND_INT64, opts)
}

func (c *Client) CallRT_TICK_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TICK_INT64, opts)
}

func (c *Client) CallRT_HISTREND_INDEX_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_HISTREND_INDEX_INT64, opts)
}

func (c *Client) CallRT_TREND_EXT_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_TREND_EXT_INT64, opts)
}

func (c *Client) CallRT_STOCKTICK_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_STOCKTICK_INT64, opts)
}

func (c *Client) CallRT_LIMITTICK_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LIMITTICK_INT64, opts)
}

func (c *Client) CallRT_MAJORINDEXTICK_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXTICK_INT64, opts)
}

func (c *Client) CallRT_VALUE_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_VALUE_INT64, opts)
}

func (c *Client) CallRT_STOCKTICK_SEARCH_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_STOCKTICK_SEARCH_INT64, opts)
}

func (c *Client) CallRT_MINUTE_TICK_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MINUTE_TICK_INT64, opts)
}

func (c *Client) CallRT_LEAD_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEAD_INT64, opts)
}

func (c *Client) CallRT_MAJORINDEXTREND_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_MAJORINDEXTREND_INT64, opts)
}

func (c *Client) CallRT_LEVEL_AUTOPUSH_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_AUTOPUSH_INT64, opts)
}

func (c *Client) CallRT_AUTOPUSH_EXT_INT64(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_AUTOPUSH_EXT_INT64, opts)
}

func (c *Client) CallRT_LEVEL_TOTALMAX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_TOTALMAX, opts)
}

func (c *Client) CallRT_LEVEL_SINGLEMAX(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL_SINGLEMAX, opts)
}

func (c *Client) CallRT_ZIPDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_ZIPDATA, opts)
}

func (c *Client) CallRT_COMPASKDATA(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_COMPASKDATA, opts)
}

func (c *Client) CallRT_END(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_END, opts)
}

func (c *Client) CallRT_LEVEL2_SHWTDL_TD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHWTDL_TD, opts)
}

func (c *Client) CallRT_LEVEL2_SHKZ_TD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHKZ_TD, opts)
}

func (c *Client) CallRT_LEVEL2_SHZB_TD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SHZB_TD, opts)
}

func (c *Client) CallRT_LEVEL2_SZZB_TD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZZB_TD, opts)
}

func (c *Client) CallRT_LEVEL2_SZWTDL_TD(ctx context.Context, opts FunctionCallOptions) (*Response, error) {
	return c.callFunction(ctx, RT_LEVEL2_SZWTDL_TD, opts)
}
