-- 充值统一按 1 元 = 10 积分换算，移除套餐折扣导致的价差。
UPDATE credit_packages SET credits = price_cents / 10;
UPDATE credit_packages SET tag = '' WHERE tag LIKE '省%';
