package com.nirvana.prd.sms.demo.net;

import com.google.gson.annotations.SerializedName;

public class GetSmsAuthTokensResponse extends PopResponse{
    @SerializedName("Data")
    private SmsAuthTokensResultData data;

    public SmsAuthTokensResultData getData() {
        return data;
    }

    public void setData(SmsAuthTokensResultData data) {
        this.data = data;
    }

    @Override
    public String toString() {
        return "GetSmsAuthTokensResponse{" +
                "data=" + data +
                '}';
    }
}
