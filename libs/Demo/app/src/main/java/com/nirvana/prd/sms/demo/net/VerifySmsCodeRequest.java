package com.nirvana.prd.sms.demo.net;

import com.google.gson.annotations.SerializedName;

public class VerifySmsCodeRequest extends PopRequest {
    {
        action = "VerifySmsCode";
    }


    @SerializedName("SmsCode")
    private String smsCode;

    @SerializedName("PhoneNumber")
    private String phoneNumber;

    @SerializedName("SmsToken")
    private String smsToken;

    public String getSmsCode() {
        return smsCode;
    }

    public void setSmsCode(String smsCode) {
        this.smsCode = smsCode;
    }

    public String getPhoneNumber() {
        return phoneNumber;
    }

    public void setPhoneNumber(String phoneNumber) {
        this.phoneNumber = phoneNumber;
    }

    public String getSmsToken() {
        return smsToken;
    }

    public void setSmsToken(String smsToken) {
        this.smsToken = smsToken;
    }
}
