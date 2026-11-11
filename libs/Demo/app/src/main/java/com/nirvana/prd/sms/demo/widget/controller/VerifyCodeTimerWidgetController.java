package com.nirvana.prd.sms.demo.widget.controller;

import android.os.Handler;
import android.os.Looper;

import java.util.Map;
import java.util.Timer;
import java.util.TimerTask;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicBoolean;

public class VerifyCodeTimerWidgetController {
    public static final Map<String, Info> sInfos = new ConcurrentHashMap<>();
    /**
     * 默认60s
     */
    private long mDuration = 60 * 1000;

    /**
     * 暂存信息
     */
    private Info mInfo = null;

    private String mControllerId = null;

    private AtomicBoolean mIsRunning = new AtomicBoolean(false);
    private Timer mTimer;
    private Handler mHandler = new Handler(Looper.getMainLooper());
    private OnTimerUpdateCallback mCallback;

    /**
     * 同一个组件的controllerId不能变，否则会时间无法继续
     * @param controllerId
     */
    public VerifyCodeTimerWidgetController(String controllerId) {
        mControllerId = controllerId;
        mInfo = sInfos.get(controllerId);
    }

    public void setDuration(long duration) {
        mDuration = duration;
    }

    public void setCallback(OnTimerUpdateCallback callback) {
        mCallback = callback;
    }

    /**
     * 开始计时
     */
    public void startTimer() {
        if(mIsRunning.compareAndSet(false, true)) {
            if (mInfo == null) {
                mInfo = new Info();
                mInfo.mStartTime = System.currentTimeMillis();
                mInfo.mEndTime = mInfo.mStartTime + mDuration;
                sInfos.put(mControllerId, mInfo);
            }
            TimerTask timerTask = new TimerTask() {
                @Override
                public void run() {
                    long current = System.currentTimeMillis();
                    final OnTimerUpdateCallback callback= mCallback;
                    final int time = (int) ((mDuration - (current - mInfo.mStartTime)) / 1000);
                    if(time <= 0) {
                        resetTimer();
                        if(callback != null) {
                            mHandler.post(() -> {
                                callback.onTimerEnding();
                            });
                        }
                        return;
                    }
                    if(callback != null) {
                        mHandler.post(() -> {
                            callback.onTimeUpdater(time);
                        });
                    }
                }
            };
            mTimer = new Timer();
            mTimer.schedule(timerTask, 1000, 1000);
        }
    }

    private void stopTimer() {
        if(mTimer != null) {
            mTimer.cancel();
            mTimer = null;
        }
        mIsRunning.set(false);
    }

    /**
     * 暂停计时
     */
    public void pauseTimer() {
        if(mIsRunning.compareAndSet(true, false)) {
            stopTimer();
        }
    }

    /**
     * 重置计时
     */
    public void resetTimer() {
        stopTimer();
        mInfo = null;
    }

    /**
     * 是否在计时
     * @return
     */
    public boolean isOnTimer() {
        return mIsRunning.get() == true && mInfo != null && mInfo.mEndTime > System.currentTimeMillis();
    }


    public static class Info {
        /**
         * 开始时间
         */
        long mStartTime;

        /**
         * 结束时间
         */
        long mEndTime;
    }

    public interface  OnTimerUpdateCallback {
        void onTimeUpdater(long time);

        void onTimerEnding();
    }

}
