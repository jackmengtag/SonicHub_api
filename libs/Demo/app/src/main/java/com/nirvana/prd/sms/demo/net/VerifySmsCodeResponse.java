package com.nirvana.prd.sms.demo.net;

import com.google.gson.annotations.SerializedName;

public class VerifySmsCodeResponse extends PopResponse{
    @SerializedName("Data")
    private boolean data;

    public boolean isData() {
        return data;
    }

    public void setData(boolean data) {
        this.data = data;
    }

    @Override
    public String toString() {
        return "VerifySmsCodeResponse{" +
                "data=" + data +
                '}';
    }
}
