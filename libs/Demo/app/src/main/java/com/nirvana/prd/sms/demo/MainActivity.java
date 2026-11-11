package com.nirvana.prd.sms.demo;

import android.app.Activity;
import android.app.ProgressDialog;
import android.graphics.Color;
import android.os.Bundle;
import androidx.annotation.NonNull;
import androidx.annotation.Nullable;
import androidx.appcompat.widget.AppCompatSpinner;
import android.text.SpannableString;
import android.text.Spanned;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.style.ClickableSpan;
import android.util.Log;
import android.view.View;
import android.widget.ArrayAdapter;
import android.widget.EditText;
import android.widget.TextView;
import android.widget.Toast;

import com.nirvana.prd.sms.auth.ResultCode;
import com.nirvana.prd.sms.auth.SmsAuthHelper;
import com.nirvana.prd.sms.auth.TokenUpdater;
import com.nirvana.prd.sms.auth.Tokens;
import com.nirvana.prd.sms.demo.net.HttpRequestUtils;
import com.nirvana.prd.sms.demo.net.VerifySmsCodeResponse;
import com.nirvana.prd.sms.demo.widget.VerifyCodeTimerWidget;
import com.nirvana.prd.sms.demo.widget.controller.VerifyCodeTimerWidgetController;

public class MainActivity extends Activity implements TokenUpdater {
    private VerifyCodeTimerWidgetController mVerifyCodeTimerWidgetController;
    private VerifyCodeTimerWidget mVerifyCodeTimerWidget;
    private AppCompatSpinner mCountrySpinner;
    private SmsAuthHelper mSmsAuthHelper;
    private EditText mPhoneNumberEt;
    private EditText mSmsCodeEt;
    private ProgressDialog mProgressDialog;
    private String mSmsToken;


    @Override
    protected void onCreate(@Nullable Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        mPhoneNumberEt = findViewById(R.id.et_phone_number);
        mSmsCodeEt = findViewById(R.id.et_verify_code);
        initSmsAuthHelper();
        initProtocolText();
        initCountryCodeSelector();
        initSendVerifyCodeWidget();
        initLoginBtn();
    }

    private void initLoginBtn() {
        findViewById(R.id.btn_login).setOnClickListener(v -> {
            String phoneNumber = mPhoneNumberEt.getText().toString();
            String smsCode = mSmsCodeEt.getText().toString();
            if (TextUtils.isEmpty(phoneNumber)
                    || TextUtils.isEmpty(smsCode)
                    || TextUtils.isEmpty(mSmsToken)) {
                Toast.makeText(getApplicationContext(), "请先填写手机号或者获取验证码", Toast.LENGTH_SHORT).show();
                return;
            }
            //对结果进行解析，登录成功则跳转到成功页面
            showLoadingDialog("正在登录");
            //使用验证码进行验证登录
            ExecutorManager.submit(() -> {
                VerifySmsCodeResponse response = HttpRequestUtils.verifyCode(phoneNumber,
                        mSmsToken, smsCode);
                runOnUiThread(()->{
                    if(response != null && response.isData()) {
                        Toast.makeText(MainActivity.this, "登录成功", Toast.LENGTH_SHORT).show();
                    }else {
                        Toast.makeText(MainActivity.this, "登录失败:"+response.getMessage(), Toast.LENGTH_SHORT).show();
                    }
                    hideLoadingDialog();
                });
            });
        });
    }

    private void initSmsAuthHelper() {
        /*
         特别注意！！！特别注意！！！特别注意！！！
         特别注意！！！特别注意！！！特别注意！！！
         特别注意！！！特别注意！！！特别注意！！！
         方案号Demo为了测试简单，直接写死了，实际开发时建议从服务端获取，防止后面方案号需要更换。
         */
        mSmsAuthHelper = new SmsAuthHelper(this, BuildConfig.SCENE_CODE);
        mSmsAuthHelper.setTokenUpdater(this);
    }

    private void initSendVerifyCodeWidget() {
        mVerifyCodeTimerWidgetController = new VerifyCodeTimerWidgetController(MainActivity.class.getName());
        mVerifyCodeTimerWidget = findViewById(R.id.btn_send_verify_code);
        mVerifyCodeTimerWidget.setOnClickListener(v -> {
            String countryCodeStr = mCountrySpinner.getSelectedItem().toString();
            Integer countryCode = Integer.parseInt(countryCodeStr.replace("+", ""));
            String phoneNumber = mPhoneNumberEt.getText().toString();
            if (TextUtils.isEmpty(phoneNumber)) {
                Toast.makeText(MainActivity.this, "请输入手机号", Toast.LENGTH_SHORT).show();
                return;
            }

            showLoadingDialog("正在请求验证码");
            mSmsAuthHelper.sendVerifyCode(countryCode, phoneNumber, ret -> {
                        Log.e("xxffc", "Ret:" + ret.toString());
                        hideLoadingDialog();
                        if (ret.getCode() == ResultCode.CODE_SUCCESS) {
                            mVerifyCodeTimerWidget.setClickable(false);
                            mVerifyCodeTimerWidgetController.startTimer();
                            mSmsToken = ret.getSmsVerifyToken();
                        }else {
                            runOnUiThread(()->{
                                Toast.makeText(MainActivity.this, "验证码发送失败："+ret.getMsg(), Toast.LENGTH_SHORT).show();
                            });
                        }
                    }, 5000
            );
        });
    }

    @Override
    protected void onResume() {
        super.onResume();
        mVerifyCodeTimerWidget.bindController(mVerifyCodeTimerWidgetController);
    }

    @Override
    protected void onPause() {
        super.onPause();
        mVerifyCodeTimerWidget.unBindController();
    }

    private void initCountryCodeSelector() {
        mCountrySpinner = findViewById(R.id.spinner_country_selector);
        ArrayAdapter<CharSequence> arrayAdapter = ArrayAdapter.createFromResource(this,
                R.array.country_array, R.layout.spinner_list_item);
        arrayAdapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);
        mCountrySpinner.setAdapter(arrayAdapter);
    }

    private void initProtocolText() {
        SpannableString spannableString = new SpannableString("同意《用户隐私协议》");
        ClickableSpan clickableSpan = new ClickableSpan() {
            @Override
            public void onClick(@NonNull View widget) {
                //
            }

            @Override
            public void updateDrawState(TextPaint ds) {
                super.updateDrawState(ds);
                ds.setUnderlineText(false);
                ds.setColor(Color.parseColor("#1890FF"));
            }
        };
        spannableString.setSpan(clickableSpan, 3, spannableString.length(), Spanned.SPAN_EXCLUSIVE_INCLUSIVE);
        ((TextView) findViewById(R.id.tv_protocol)).setText(spannableString);
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        mSmsAuthHelper.destroy();
    }

    @Override
    public Tokens updateToken() {
        return HttpRequestUtils.requestTokens(MainActivity.this);
    }

    public void showLoadingDialog(String hint) {
        if (mProgressDialog == null) {
            mProgressDialog = new ProgressDialog(this);
            mProgressDialog.setProgressStyle(ProgressDialog.STYLE_SPINNER);
        }
        mProgressDialog.setMessage(hint);
        mProgressDialog.setCancelable(true);
        mProgressDialog.show();
    }

    public void hideLoadingDialog() {
        if (mProgressDialog != null) {
            mProgressDialog.dismiss();
        }
    }
}
