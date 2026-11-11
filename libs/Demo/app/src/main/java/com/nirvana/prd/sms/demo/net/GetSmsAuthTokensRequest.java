package com.nirvana.prd.sms.demo.net;

import com.google.gson.annotations.SerializedName;
public class GetSmsAuthTokensRequest extends PopRequest {
    {
        action = "GetSmsAuthTokens";
    }
    @SerializedName("SceneCode")
    private String sceneCode;

    @SerializedName("OsType")
    private String osType="Android";

    @SerializedName("PackageName")
    private String packageName;

    @SerializedName("SignName")
    private String appSignature;

    @SerializedName("Expire")
    private long expireTime = 3600;//单位是秒

    @SerializedName("SmsCodeExpire")
    private long smsCodeValidTime = 60;//单位是秒

    @SerializedName("SmsTemplateCode")
    private String smsTemplateCode;

    public String getSceneCode() {
        return sceneCode;
    }

    public void setSceneCode(String sceneCode) {
        this.sceneCode = sceneCode;
    }

    public String getOsType() {
        return osType;
    }

    public String getPackageName() {
        return packageName;
    }

    public void setPackageName(String packageName) {
        this.packageName = packageName;
    }

    public String getAppSignature() {
        return appSignature;
    }

    public void setAppSignature(String appSignature) {
        this.appSignature = appSignature;
    }

    public long getExpireTime() {
        return expireTime;
    }

    public void setExpireTime(long expireTime) {
        this.expireTime = expireTime;
    }

    public String getSmsTemplateCode() {
        return smsTemplateCode;
    }

    public void setSmsTemplateCode(String smsTemplateCode) {
        this.smsTemplateCode = smsTemplateCode;
    }

    public long getSmsCodeValidTime() {
        return smsCodeValidTime;
    }

    public void setSmsCodeValidTime(long smsCodeValidTime) {
        this.smsCodeValidTime = smsCodeValidTime;
    }
}
