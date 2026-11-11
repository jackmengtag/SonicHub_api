package com.nirvana.prd.sms.demo.net;

import com.google.gson.annotations.SerializedName;

public class PopResponse {
    @SerializedName("RequestId")
    private String requestId;
    @SerializedName("HostId")
    private String hostId;
    @SerializedName("Code")
    private String code;
    @SerializedName("Message")
    private String message;
    @SerializedName("Recommend")
    private String recommend;

    public String getRequestId() {
        return requestId;
    }

    public void setRequestId(String requestId) {
        this.requestId = requestId;
    }

    public String getHostId() {
        return hostId;
    }

    public void setHostId(String hostId) {
        this.hostId = hostId;
    }

    public String getCode() {
        return code;
    }

    public void setCode(String code) {
        this.code = code;
    }

    public String getMessage() {
        return message;
    }

    public void setMessage(String message) {
        this.message = message;
    }

    public String getRecommend() {
        return recommend;
    }

    public void setRecommend(String recommend) {
        this.recommend = recommend;
    }

    @Override
    public String toString() {
        return "PopResponse{" +
                "requestId='" + requestId + '\'' +
                ", hostId='" + hostId + '\'' +
                ", code='" + code + '\'' +
                ", message='" + message + '\'' +
                ", recommend='" + recommend + '\'' +
                '}';
    }
}
