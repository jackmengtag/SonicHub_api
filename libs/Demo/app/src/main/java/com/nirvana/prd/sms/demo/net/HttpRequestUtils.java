package com.nirvana.prd.sms.demo.net;

import android.content.Context;
import android.text.TextUtils;
import android.util.Log;

import com.google.gson.Gson;
import com.nirvana.prd.sms.auth.Tokens;
import com.nirvana.prd.sms.auth.utils.PackageUtils;
import com.nirvana.prd.sms.demo.BuildConfig;

import java.text.SimpleDateFormat;
import java.util.Date;

public class HttpRequestUtils {
    /**
     * 验证码认证
     *
     * @return
     */
    public static VerifySmsCodeResponse verifyCode(String phoneNumber,
                                                   String smsToken,
                                                   String smsCode) {
        VerifySmsCodeRequest request = new VerifySmsCodeRequest();
        request.setAccessKeyId(BuildConfig.ACCESS_KEY_ID);
        request.setAccessKeySecret(BuildConfig.ACCESS_KEY_SECRET);
        request.setPhoneNumber(phoneNumber);
        request.setSmsCode(smsCode);
        request.setSmsToken(smsToken);
        String result = PopRequestUtils.post(request);
        Log.e("xxffc", "VerifyCode Response:" + result);
        if (!TextUtils.isEmpty(result)) {
            Gson gson = new Gson();
            VerifySmsCodeResponse response = gson.fromJson(result, VerifySmsCodeResponse.class);
            return response;
        }
        return null;
    }

    public static Tokens requestTokens(Context context) {
        //特别注意！！！ 特别注意！！！
        //特别注意！！！ 特别注意！！！
        //特别注意！！！ 特别注意！！！
        /*Demo为了演示方便，所以直接用ak向阿里云pop api发请求获取，实际开发中，
        应该由APP开发者应该通过自己的服务端间接向阿里云pop api发请求，并且保证APP与APP Server之间通信的数据安全*/
        GetSmsAuthTokensRequest request = new GetSmsAuthTokensRequest();
        request.setAccessKeyId(BuildConfig.ACCESS_KEY_ID);
        request.setAccessKeySecret(BuildConfig.ACCESS_KEY_SECRET);
        request.setRegionId("cn-hangzhou");
        request.setAppSignature(PackageUtils.getSign(context));
        request.setPackageName(PackageUtils.getPackageName(context));
        request.setSceneCode(BuildConfig.SCENE_CODE);
        request.setSmsTemplateCode(BuildConfig.SMS_TEMPLATE_CODE);
        request.setSmsCodeValidTime(60);
        String result = PopRequestUtils.post(request);
        Log.e("xxffc", "GetSmsAuthTokensResult:" + result);
        if (!TextUtils.isEmpty(result)) {
            Gson gson = new Gson();
            GetSmsAuthTokensResponse response = gson.fromJson(result, GetSmsAuthTokensResponse.class);
            SmsAuthTokensResultData resultData;
            if (response != null && (resultData = response.getData()) != null) {
                Tokens tokens = new Tokens();
                tokens.setExpiredTimeMills(resultData.getExpiredTime());
                tokens.setBizToken(resultData.getBizToken());
                tokens.setAccessKeyId(resultData.getStsAccessKeyId());
                tokens.setAccessKeySecret(resultData.getStsAccessKeySecret());
                tokens.setStsToken(resultData.getStsToken());
                Log.e("xxffc", "SmsAuthTokens:expiredTime:" + new SimpleDateFormat("yyyy-MM-dd hh:mm:ss.SSS").format(new Date(resultData.getExpiredTime())));
                return tokens;
            }
        }
        return null;
    }
}
