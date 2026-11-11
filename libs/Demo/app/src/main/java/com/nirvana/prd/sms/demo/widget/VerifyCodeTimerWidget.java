package com.nirvana.prd.sms.demo.widget;

import android.content.Context;
import android.util.AttributeSet;

import com.nirvana.prd.sms.demo.widget.controller.VerifyCodeTimerWidgetController;

public class VerifyCodeTimerWidget
        extends androidx.appcompat.widget.AppCompatTextView
        implements VerifyCodeTimerWidgetController.OnTimerUpdateCallback {
    private VerifyCodeTimerWidgetController mController;

    {
        setText("获取验证码");
    }

    public VerifyCodeTimerWidget(Context context) {
        super(context);
    }

    public VerifyCodeTimerWidget(Context context, AttributeSet attrs) {
        super(context, attrs);
    }

    public VerifyCodeTimerWidget(Context context, AttributeSet attrs, int defStyleAttr) {
        super(context, attrs, defStyleAttr);
    }

    public void bindController(VerifyCodeTimerWidgetController controller) {
        mController = controller;
        mController.setCallback(this);
    }

    public void unBindController() {
        mController.setCallback(null);
        mController = null;
    }


    @Override
    public void onTimeUpdater(long time) {
        setText("重新发送 "+time + "s");
    }

    @Override
    public void onTimerEnding() {
        setText("重新发送");
        setClickable(true);
    }
}
