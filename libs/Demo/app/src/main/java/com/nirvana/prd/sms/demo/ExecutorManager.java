package com.nirvana.prd.sms.demo;

import android.os.Handler;
import android.os.Looper;

import java.util.HashMap;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.Future;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;

public class ExecutorManager {
    private static final String TAG = "NIRVANA_EXECUTOR";
    private static ThreadPoolExecutor threadExecutor;
    private static Handler mMainHandler;

    static {
        threadExecutor = new ThreadPoolExecutor(Runtime.getRuntime().availableProcessors(),
                Runtime.getRuntime().availableProcessors(),
                5, TimeUnit.SECONDS, new ArrayBlockingQueue<Runnable>(10),
                new ThreadPoolExecutor.CallerRunsPolicy());
        threadExecutor.allowCoreThreadTimeOut(true);
        mMainHandler = new Handler(Looper.getMainLooper());
    }


    public static Future<?> submit(Runnable runnable) {
        return threadExecutor.submit(runnable);
    }


    public static void postMain(Runnable runnable) {
        mMainHandler.post(runnable);
    }

    public static void removeFromMain(Runnable runnable) {
        mMainHandler.removeCallbacks(runnable);
    }
}
