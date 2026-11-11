package com.nirvana.prd.sms.demo.net;

import com.google.gson.annotations.SerializedName;

public class SmsAuthTokensResultData {
    @SerializedName("BizToken")
    private String bizToken;
    @SerializedName("ExpireTime")
    private long expiredTime;
    @SerializedName("StsAccessKeyId")
    private String stsAccessKeyId;
    @SerializedName("StsAccessKeySecret")
    private String stsAccessKeySecret;
    @SerializedName("StsToken")
    private String stsToken;

    public String getBizToken() {
        return bizToken;
    }

    public void setBizToken(String bizToken) {
        this.bizToken = bizToken;
    }

    public long getExpiredTime() {
        return expiredTime;
    }

    public void setExpiredTime(long expiredTime) {
        this.expiredTime = expiredTime;
    }

    public String getStsAccessKeyId() {
        return stsAccessKeyId;
    }

    public void setStsAccessKeyId(String stsAccessKeyId) {
        this.stsAccessKeyId = stsAccessKeyId;
    }

    public String getStsAccessKeySecret() {
        return stsAccessKeySecret;
    }

    public void setStsAccessKeySecret(String stsAccessKeySecret) {
        this.stsAccessKeySecret = stsAccessKeySecret;
    }

    public String getStsToken() {
        return stsToken;
    }

    public void setStsToken(String stsToken) {
        this.stsToken = stsToken;
    }
}
