-- Dev seed for the bootstrap user (dev@local.test).
-- Wipes previous seed entities for the same user, then inserts a small
-- multi-account portfolio plus ~120 daily portfolio snapshots so the
-- dashboard history chart has data to render.
--
-- Apply via `make seed` while the api service is running. Safe to re-run.

BEGIN;

DO $$
DECLARE
    v_user_id        UUID;
    v_acc_broker     UUID := gen_random_uuid();
    v_acc_crypto     UUID := gen_random_uuid();
    v_acc_wallet     UUID := gen_random_uuid();
    v_inst_sber      UUID := gen_random_uuid();
    v_inst_lkoh      UUID := gen_random_uuid();
    v_inst_aapl      UUID := gen_random_uuid();
    v_inst_btc       UUID := gen_random_uuid();
    v_inst_eth       UUID := gen_random_uuid();
    v_inst_usd_cash  UUID := gen_random_uuid();
    v_today          DATE := CURRENT_DATE;
    v_day            INT;
    v_total          NUMERIC(24, 8);
    v_base           NUMERIC(24, 8) := 850000;
BEGIN
    SELECT id INTO v_user_id FROM users WHERE email = 'dev@local.test';
    IF v_user_id IS NULL THEN
        RAISE EXCEPTION 'bootstrap user dev@local.test not found; start api service first';
    END IF;

    DELETE FROM accounts            WHERE user_id = v_user_id;
    DELETE FROM portfolio_snapshots WHERE user_id = v_user_id;
    DELETE FROM deposits            WHERE user_id = v_user_id;
    DELETE FROM instruments         WHERE ticker IN ('SBER','LKOH','AAPL','BTC','ETH','USD');

    INSERT INTO instruments (id, ticker, asset_class, currency, name) VALUES
        (v_inst_sber,     'SBER', 'ru_stock', 'RUB', 'Сбербанк'),
        (v_inst_lkoh,     'LKOH', 'ru_stock', 'RUB', 'Лукойл'),
        (v_inst_aapl,     'AAPL', 'us_stock', 'USD', 'Apple Inc.'),
        (v_inst_btc,      'BTC',  'crypto',   'USD', 'Bitcoin'),
        (v_inst_eth,      'ETH',  'crypto',   'USD', 'Ethereum'),
        (v_inst_usd_cash, 'USD',  'cash',     'USD', 'USD cash');

    INSERT INTO prices (instrument_id, price, fetched_at) VALUES
        (v_inst_sber,     312.50,   now()),
        (v_inst_lkoh,     7100.00,  now()),
        (v_inst_aapl,     195.20,   now()),
        (v_inst_btc,      62500.00, now()),
        (v_inst_eth,      3100.00,  now()),
        (v_inst_usd_cash, 1.0,      now());

    INSERT INTO accounts (id, user_id, source_type, name) VALUES
        (v_acc_broker, v_user_id, 'manual', 'Дев брокер'),
        (v_acc_crypto, v_user_id, 'manual', 'Дев крипта'),
        (v_acc_wallet, v_user_id, 'manual', 'Дев USD-кошелёк');

    INSERT INTO positions (account_id, instrument_id, quantity) VALUES
        (v_acc_broker, v_inst_sber,     1500),
        (v_acc_broker, v_inst_lkoh,     40),
        (v_acc_broker, v_inst_aapl,     25),
        (v_acc_crypto, v_inst_btc,      0.12),
        (v_acc_crypto, v_inst_eth,      1.8),
        (v_acc_wallet, v_inst_usd_cash, 4500);

    INSERT INTO fx_rates (date, from_ccy, to_ccy, rate) VALUES
        (v_today, 'USD', 'RUB', 92.5),
        (v_today, 'RUB', 'USD', round(1.0 / 92.5, 10));

    FOR v_day IN 0..119 LOOP
        v_total := GREATEST(
            50000,
            v_base
              * (1 + 0.06 * sin(v_day::float / 18))
              + v_day::numeric * 800
              + (random() - 0.5) * 25000
        );
        INSERT INTO portfolio_snapshots (
            user_id, snapshot_date, display_currency,
            grand_total, by_asset_class, by_currency, by_account
        ) VALUES (
            v_user_id,
            v_today - (119 - v_day),
            'RUB',
            v_total,
            jsonb_build_object(
                'ru_stock', round(v_total * 0.42, 2)::text,
                'us_stock', round(v_total * 0.18, 2)::text,
                'crypto',   round(v_total * 0.28, 2)::text,
                'cash',     round(v_total * 0.12, 2)::text
            ),
            jsonb_build_object(
                'RUB', round(v_total * 0.55, 2)::text,
                'USD', round(v_total * 0.45, 2)::text
            ),
            jsonb_build_object(
                v_acc_broker::text, round(v_total * 0.55, 2)::text,
                v_acc_crypto::text, round(v_total * 0.28, 2)::text,
                v_acc_wallet::text, round(v_total * 0.17, 2)::text
            )
        );
    END LOOP;
END $$;

COMMIT;
