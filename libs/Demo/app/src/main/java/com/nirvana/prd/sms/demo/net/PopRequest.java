package com.nirvana.prd.sms.demo.net;

import android.util.Base64;

import com.google.gson.annotations.SerializedName;
import com.nirvana.prd.sms.demo.BuildConfig;

import java.io.UnsupportedEncodingException;
import java.lang.reflect.Field;
import java.net.URLEncoder;
import java.security.InvalidKeyException;
import java.security.NoSuchAlgorithmException;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Date;
import java.util.List;
import java.util.Map;
import java.util.TimeZone;
import java.util.TreeMap;
import java.util.UUID;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

public abstract class PopRequest {
    public static final SimpleDateFormat POP_REQUEST_DATE_FORMAT = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'");
    static {
        POP_REQUEST_DATE_FORMAT.setTimeZone(TimeZone.getTimeZone("O"));
    }

    @SerializedName("SignatureMethod")
    private String signatureMethod = "HMAC-SHA1";

    @SerializedName("SignatureNonce")
    private String signatureNonce = UUID.randomUUID().toString();

    @SerializedName("AccessKeyId")
    private String accessKeyId = null;

    @SerializedName("SignatureVersion")
    private String signatureVersion = "1.0";

    @SerializedName("Timestamp")
    private String timeStamp = POP_REQUEST_DATE_FORMAT.format(new Date());

    @SerializedName("Format")
    private String format = "JSON";

    @SerializedName("RegionId")
    private String regionId = "cn-hangzhou";

    @SerializedName("Version")
    private String version = "2017-05-25";

    @SerializedName("Signature")
    private String sign = null;

    @SerializedName("Action")
    protected String action = null;

    @SerializedName("SecurityToken")
    private String stsToken;

    @SerializedName("AccessSecret")
    private String accessKeySecret;

    private String baseUrl = BuildConfig.SERVER_URL;

    String buildSign() {
        List<Field> fields = getAllDeclaredFields(this.getClass());
        StringBuilder stringToSign = new StringBuilder();
        StringBuilder canonicalQueryString = new StringBuilder();
        TreeMap<String, Object> canonicalMap = new TreeMap<>();
        for (Field field:fields) {
            SerializedName serializedName = field.getAnnotation(SerializedName.class);
            if (serializedName != null) {
                String fieldKey = serializedName.value();
                if ("Signature" == fieldKey) {
                    continue;
                }
                field.setAccessible(true);
                Object value = null;
                try {
                    value = field.get(this);
                    if(value != null) {
                        canonicalMap.put(fieldKey, value);
                    }
                } catch (IllegalAccessException e) {
                    e.printStackTrace();
                }
            }
        }
        for (Map.Entry<String,Object> entry:canonicalMap.entrySet()) {
            canonicalQueryString.append("&")
                    .append(specialUrlEncode(entry.getKey()))
                    .append("=").append(specialUrlEncode(entry.getValue() == null?"":entry.getValue().toString()));

        }

        stringToSign.append("POST");
        stringToSign.append("&");
        stringToSign.append(specialUrlEncode("/"));
        stringToSign.append("&");
        stringToSign.append(specialUrlEncode(canonicalQueryString.toString().substring(1)));
        sign = specialUrlEncode(sign(stringToSign.toString(), accessKeySecret + "&"));
//        Log.d("xxffc", "StringToSign:"+stringToSign);
//        Log.d("xxffc", "Signature:"+sign);
        return canonicalQueryString+"&Signature="+sign;
    }


    private List<Field> getAllDeclaredFields(Class clz) {
        List<Field> fields = new ArrayList();
        fields.addAll(Arrays.asList(clz.getDeclaredFields()));
        if(!clz.getName().equals(PopRequest.class.getName())) {
            Class superclass = clz.getSuperclass();
            if (superclass != null && !superclass.getName().equals(Object.class.getName())) {
                fields.addAll(getAllDeclaredFields(superclass));
            }
        }
        return fields;
    }

    public static String specialUrlEncode(String value) {
        try {
            return URLEncoder.encode(value, "UTF-8").replace("+", "%20").replace("*", "%2A").replace("%7E", "~");
        } catch (Exception e) {
            return "";
        }
    }

    private String sign(String stringToSign, String accessSecret) {
        try {
            Mac mac = Mac.getInstance("HmacSHA1");
            mac.init(new SecretKeySpec(accessSecret.getBytes("utf-8"), "HmacSHA1"));
            byte[] signData = mac.doFinal(stringToSign.getBytes("utf-8"));
            return Base64.encodeToString(signData, Base64.NO_WRAP);
        } catch (NoSuchAlgorithmException var5) {
            throw new IllegalArgumentException(var5.toString());
        } catch (InvalidKeyException var7) {
            throw new IllegalArgumentException(var7.toString());
        } catch (UnsupportedEncodingException e) {
            e.printStackTrace();
        }
        return null;
    }

    public void setAccessKeyId(String accessKeyId) {
        this.accessKeyId = accessKeyId;
    }

    public void setStsToken(String stsToken) {
        this.stsToken = stsToken;
    }

    public void setAccessKeySecret(String accessKeySecret) {
        this.accessKeySecret = accessKeySecret;
    }

    public void setRegionId(String regionId) {
        this.regionId = regionId;
    }

    public void setVersion(String version) {
        this.version = version;
    }

    public void setBaseUrl(String baseUrl) {
        this.baseUrl = baseUrl;
    }

    public String getBaseUrl() {
        return baseUrl;
    }
}