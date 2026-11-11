package com.nirvana.prd.sms.demo.net;

import android.util.Log;

import com.nirvana.tools.jsoner.JSONUtils;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.SocketTimeoutException;
import java.net.URL;
import java.security.KeyManagementException;
import java.security.NoSuchAlgorithmException;
import java.security.cert.CertificateException;
import java.security.cert.X509Certificate;

import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

public class PopRequestUtils {
    public static final String POP_REQUEST_CODE_SUCCESS = "OK";


    public static <T extends PopRequest> String post(T request) {
        if (request.getBaseUrl().startsWith("https")) {
            return postHttps(request.getBaseUrl(), request);
        } else {
            return postHttp(request.getBaseUrl(), request);
        }
    }

    private static <T extends PopRequest> String postHttps(String urlPath, T request) {
        HttpsURLConnection conn = null;
        OutputStream out = null;
        InputStream is = null;
        InputStreamReader isr = null;
        BufferedReader br = null;
        String rsp = null;
        StringBuffer buffer = null;
        try {
            String params = request.buildSign();
            URL url = new URL(urlPath);
            conn = (HttpsURLConnection) url.openConnection();
            conn.setDoOutput(true);
            conn.setDoInput(true);
            conn.setUseCaches(false);
            conn.setRequestMethod("POST");
            conn.setConnectTimeout(5000);
            conn.setReadTimeout(5000);
            try {
                SSLContext sslContext = SSLContext.getInstance("SSL");
                X509TrustManager x509TrustManager = new X509TrustManager() {
                    @Override
                    public void checkClientTrusted(X509Certificate[] chain, String authType) throws CertificateException {

                    }

                    @Override
                    public void checkServerTrusted(X509Certificate[] chain, String authType) throws CertificateException {

                    }

                    @Override
                    public X509Certificate[] getAcceptedIssuers() {
                        return new X509Certificate[0];
                    }
                };
                sslContext.init(null, new TrustManager[] {x509TrustManager}, null);
                conn.setSSLSocketFactory(sslContext.getSocketFactory());
            } catch (NoSuchAlgorithmException e) {
                e.printStackTrace();
            } catch (KeyManagementException e) {
                e.printStackTrace();
            }
            conn.setRequestProperty("Host", url.getHost());
            conn.setRequestProperty("Accept", "text/text,text/javascript");
            conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded;charset=utf-8");
            conn.connect();
            out = conn.getOutputStream();
            Log.e("xxffc", "Params:"+params);
            out.write(params.getBytes("utf-8"));
            int code = conn.getResponseCode();
            if(code == HttpsURLConnection.HTTP_OK) {
                is = conn.getInputStream();
            }else {
                is = conn.getErrorStream();
            }
            isr = new InputStreamReader(is, "utf-8");
            br = new BufferedReader(isr);
            buffer = new StringBuffer();
            String line = null;

            while ((line = br.readLine()) != null) {
                buffer.append(line);
            }
            rsp = new String(buffer);
        } catch (SocketTimeoutException e) {
            e.printStackTrace();
            //todo:打印错误信息
            return Log.getStackTraceString(e);
        } catch (IOException e) {
            e.printStackTrace();
            //todo:打印错误信息
            return Log.getStackTraceString(e);
        } finally {
            try {
                if (is != null) {
                    is.close();
                }

                if (isr != null) {
                    isr.close();
                }

                if (br != null) {
                    br.close();
                }

                if (out != null) {
                    out.close();
                }
                if (conn != null) {
                    conn.disconnect();
                }
            } catch (Throwable e) {
                //todo:打印错误信息
                e.printStackTrace();
            }
        }
        return rsp;
    }

    private static <T extends PopRequest> String postHttp(String urlPath, T request) {
        HttpURLConnection conn = null;
        OutputStream out = null;
        InputStream is = null;
        InputStreamReader isr = null;
        BufferedReader br = null;
        String rsp = null;
        StringBuffer buffer = null;
        try {
            request.buildSign();
            URL url = new URL(urlPath);
            conn = (HttpURLConnection) url.openConnection();
            conn.setDoOutput(true);
            conn.setDoInput(true);
            conn.setUseCaches(false);
            conn.setRequestMethod("POST");
            conn.setConnectTimeout(3000);
            conn.setReadTimeout(3000);
            conn.setRequestProperty("Host", url.getHost());
            conn.setRequestProperty("Accept", "text/xml,text/javascript");
            conn.setRequestProperty("User-Agent", "top-sdk-java");
            conn.setRequestProperty("Content-Type", "application/x-www-form-urlencoded;charset=utf-8");
            conn.connect();
            out = conn.getOutputStream();
            out.write(JSONUtils.toJson(request, null).toString().getBytes("utf-8"));

            //读取服务器端返回的内容
            is = conn.getInputStream();
            isr = new InputStreamReader(is, "utf-8");
            br = new BufferedReader(isr);
            buffer = new StringBuffer();
            String line = null;
            while ((line = br.readLine()) != null) {
                buffer.append(line);
            }
            rsp = new String(buffer);
        } catch (SocketTimeoutException e) {
            //todo:打印错误信息
            return Log.getStackTraceString(e);
        } catch (IOException e) {
            //todo:打印错误信息
            return Log.getStackTraceString(e);
        } finally {
            try {
                if (is != null) {
                    is.close();
                }

                if (isr != null) {
                    isr.close();
                }

                if (br != null) {
                    br.close();
                }

                if (out != null) {
                    out.close();
                }
                if (conn != null) {
                    conn.disconnect();
                }
            } catch (Throwable e) {
                //todo:打印错误信息
                e.printStackTrace();
            }
        }
        return rsp;
    }
}
