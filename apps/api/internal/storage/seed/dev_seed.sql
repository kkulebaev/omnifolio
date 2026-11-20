-- Dev seed for the bootstrap user (dev@local.test).
-- Wipes previous seed entities for the same user, then inserts a multi-account
-- portfolio, ~14 months of deposits/withdrawals in RUB and USD, daily FX
-- history and 420 daily portfolio snapshots.
--
-- Snapshots are reconstructed BACKWARDS from today's positions so that the
-- history, the cash-flow journal and the live /portfolio value agree with each
-- other — otherwise TWR/XIRR on the dashboard come out as nonsense. A flow
-- dated d is reflected starting from snapshot d+1, matching the convention in
-- internal/returns (snapshots are taken at 09:00 UTC, before same-day flows).
--
-- Only the bootstrap user's own rows are deleted. Shared data is upserted:
-- global instruments are reused, seed prices overwrite theirs, and FX rates
-- are added only where a date has none (real CBR rates win).
--
-- Apply via `make seed` while the api service is running. Safe to re-run.

BEGIN;

CREATE FUNCTION pg_temp.seed_instrument(p_ticker TEXT, p_class TEXT, p_ccy TEXT, p_name TEXT)
RETURNS UUID LANGUAGE sql AS $fn$
    INSERT INTO instruments (id, ticker, asset_class, currency, name)
    VALUES (gen_random_uuid(), p_ticker, p_class, p_ccy, p_name)
    ON CONFLICT (LOWER(ticker), asset_class) WHERE user_id IS NULL
        DO UPDATE SET ticker = instruments.ticker
    RETURNING id;
$fn$;

DO $$
DECLARE
    v_user_id        UUID;
    v_acc_broker     UUID := gen_random_uuid();
    v_acc_crypto     UUID := gen_random_uuid();
    v_acc_wallet     UUID := gen_random_uuid();
    v_inst_sber      UUID;
    v_inst_lkoh      UUID;
    v_inst_aapl      UUID;
    v_inst_btc       UUID;
    v_inst_eth       UUID;
    v_inst_usd_cash  UUID;
    v_today          DATE := CURRENT_DATE;
    v_days           CONSTANT INT := 420;
    v_fx_days        CONSTANT INT := 460;

    -- USD bucket split, fixed to today's position mix so per-class/per-account
    -- breakdowns in history stay proportional.
    v_usd_aapl       NUMERIC := 20 * 195.20;
    v_usd_crypto     NUMERIC := 0.08 * 62500 + 0.8 * 3100;
    v_usd_cash       NUMERIC := 1500;
    v_usd_total      NUMERIC;

    v_rub            NUMERIC;
    v_usd            NUMERIC;
    v_rate           NUMERIC;
    v_total          NUMERIC;
    v_flow_rub       NUMERIC;
    v_flow_usd       NUMERIC;
    v_date           DATE;
    v_m              INT;
    i                INT;
BEGIN
    SELECT id INTO v_user_id FROM users WHERE email = 'dev@local.test';
    IF v_user_id IS NULL THEN
        RAISE EXCEPTION 'bootstrap user dev@local.test not found; start api service first';
    END IF;

    PERFORM setseed(0.42);

    DELETE FROM accounts            WHERE user_id = v_user_id;
    DELETE FROM portfolio_snapshots WHERE user_id = v_user_id;
    DELETE FROM deposits            WHERE user_id = v_user_id;

    -- Global instruments are shared with synced accounts and other users, so
    -- reuse existing rows instead of deleting them (positions RESTRICT the
    -- delete, and a cascade would drop real prices and external ids).
    v_inst_sber     := pg_temp.seed_instrument('SBER', 'ru_stock', 'RUB', 'Сбербанк');
    v_inst_lkoh     := pg_temp.seed_instrument('LKOH', 'ru_stock', 'RUB', 'Лукойл');
    v_inst_aapl     := pg_temp.seed_instrument('AAPL', 'us_stock', 'USD', 'Apple Inc.');
    v_inst_btc      := pg_temp.seed_instrument('BTC',  'crypto',   'USD', 'Bitcoin');
    v_inst_eth      := pg_temp.seed_instrument('ETH',  'crypto',   'USD', 'Ethereum');
    v_inst_usd_cash := pg_temp.seed_instrument('USD',  'cash',     'USD', 'USD cash');

    -- Prices are overwritten on purpose: history is reconstructed from these
    -- exact values, so a different live price would break the reconciliation.
    INSERT INTO prices (instrument_id, price, fetched_at) VALUES
        (v_inst_sber,     312.50,   now()),
        (v_inst_lkoh,     7100.00,  now()),
        (v_inst_aapl,     195.20,   now()),
        (v_inst_btc,      62500.00, now()),
        (v_inst_eth,      3100.00,  now()),
        (v_inst_usd_cash, 1.0,      now())
    ON CONFLICT (instrument_id) DO UPDATE
        SET price = EXCLUDED.price, fetched_at = EXCLUDED.fetched_at;

    INSERT INTO accounts (id, user_id, source_type, name) VALUES
        (v_acc_broker, v_user_id, 'manual', 'Дев брокер'),
        (v_acc_crypto, v_user_id, 'manual', 'Дев крипта'),
        (v_acc_wallet, v_user_id, 'manual', 'Дев USD-кошелёк');

    INSERT INTO positions (account_id, instrument_id, quantity) VALUES
        (v_acc_broker, v_inst_sber,     3000),
        (v_acc_broker, v_inst_lkoh,     100),
        (v_acc_broker, v_inst_aapl,     20),
        (v_acc_crypto, v_inst_btc,      0.08),
        (v_acc_crypto, v_inst_eth,      0.8),
        (v_acc_wallet, v_inst_usd_cash, 1500);

    -- CBR publishes X→RUB only; the api derives RUB→X by inversion.
    FOR i IN 0..v_fx_days - 1 LOOP
        INSERT INTO fx_rates (date, from_ccy, to_ccy, rate) VALUES
            (v_today - i, 'USD', 'RUB', round((92.5 - 6 * sin(i / 70.0) + (random() - 0.5) * 0.8)::numeric, 4)),
            (v_today - i, 'EUR', 'RUB', round((99.8 - 5 * sin(i / 60.0) + (random() - 0.5) * 0.8)::numeric, 4))
        ON CONFLICT (date, from_ccy, to_ccy) DO NOTHING;
    END LOOP;

    -- Initial capital lands before the first snapshot, so TWR starts from an
    -- already-funded portfolio while XIRR still sees the full cash-flow history.
    INSERT INTO deposits (id, user_id, date, kind, currency, amount) VALUES
        (gen_random_uuid(), v_user_id, v_today - 430, 'deposit',    'RUB', 900000),
        (gen_random_uuid(), v_user_id, v_today - 428, 'deposit',    'USD', 7000),
        (gen_random_uuid(), v_user_id, v_today - 300, 'deposit',    'USD', 1000),
        (gen_random_uuid(), v_user_id, v_today - 200, 'withdrawal', 'RUB', 100000),
        (gen_random_uuid(), v_user_id, v_today - 120, 'deposit',    'USD', 1000.50);
    FOR v_m IN 0..13 LOOP
        v_date := (date_trunc('month', v_today) - make_interval(months => v_m))::date + 9;
        IF v_date <= v_today THEN
            INSERT INTO deposits (id, user_id, date, kind, currency, amount)
            VALUES (gen_random_uuid(), v_user_id, v_date, 'deposit', 'RUB', 50000);
        END IF;
    END LOOP;

    v_usd_total := v_usd_aapl + v_usd_crypto + v_usd_cash;
    v_rub := 3000 * 312.50 + 100 * 7100;
    v_usd := v_usd_total;

    FOR i IN 0..v_days - 1 LOOP
        v_date := v_today - i;

        -- Flows dated d first show up in snapshot d+1, so peel them off before
        -- writing snapshot d.
        SELECT COALESCE(SUM(CASE WHEN kind = 'deposit' THEN amount ELSE -amount END) FILTER (WHERE currency = 'RUB'), 0),
               COALESCE(SUM(CASE WHEN kind = 'deposit' THEN amount ELSE -amount END) FILTER (WHERE currency = 'USD'), 0)
          INTO v_flow_rub, v_flow_usd
          FROM deposits
         WHERE user_id = v_user_id AND date = v_date;
        v_rub := v_rub - v_flow_rub;
        v_usd := v_usd - v_flow_usd;

        SELECT rate INTO v_rate FROM fx_rates WHERE date = v_date AND from_ccy = 'USD' AND to_ccy = 'RUB';
        v_total := v_rub + v_usd * v_rate;

        INSERT INTO portfolio_snapshots (
            user_id, snapshot_date, display_currency,
            grand_total, by_asset_class, by_currency, by_account
        ) VALUES (
            v_user_id, v_date, 'RUB', round(v_total, 2),
            jsonb_build_object(
                'ru_stock', round(v_rub, 2)::text,
                'us_stock', round(v_usd * v_usd_aapl   / v_usd_total * v_rate, 2)::text,
                'crypto',   round(v_usd * v_usd_crypto / v_usd_total * v_rate, 2)::text,
                'cash',     round(v_usd * v_usd_cash   / v_usd_total * v_rate, 2)::text
            ),
            jsonb_build_object(
                'RUB', round(v_rub, 2)::text,
                'USD', round(v_usd, 2)::text
            ),
            jsonb_build_object(
                v_acc_broker::text, round(v_rub + v_usd * v_usd_aapl / v_usd_total * v_rate, 2)::text,
                v_acc_crypto::text, round(v_usd * v_usd_crypto / v_usd_total * v_rate, 2)::text,
                v_acc_wallet::text, round(v_usd * v_usd_cash   / v_usd_total * v_rate, 2)::text
            )
        );

        v_rub := v_rub / exp(0.00033 + 0.004 * cos(i / 25.0) / 25 + (random() - 0.5) * 0.012)::numeric;
        v_usd := v_usd / exp(0.00088 + 0.010 * cos(i / 18.0) / 18 + (random() - 0.5) * 0.035)::numeric;
    END LOOP;
END $$;

COMMIT;
