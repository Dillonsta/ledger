#!/bin/sh
# ledger API smoke test (run against 127.0.0.1:18300)
set -e
BASE=http://127.0.0.1:18300
PASS=0; FAIL=0

check() { # name expected actual
  if [ "$2" = "$3" ]; then PASS=$((PASS+1)); echo "PASS  $1";
  else FAIL=$((FAIL+1)); echo "FAIL  $1  expected=[$2] actual=[$3]"; fi
}

# 1. wrong password -> 401
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"wrong"}')
check "login-wrong-pass-401" 401 "$code"

# 2. login ok
login=$(curl -s -X POST $BASE/api/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin123"}')
token=$(echo "$login" | grep -o '"token":"[^"]*' | cut -d'"' -f4)
[ -n "$token" ] && check "login-returns-token" ok ok || check "login-returns-token" ok "EMPTY"

AUTH="Authorization: Bearer $token"

# 3. no token -> 401
code=$(curl -s -o /dev/null -w '%{http_code}' $BASE/api/tx)
check "tx-no-token-401" 401 "$code"

# 4. categories seeded
cats=$(curl -s -H "$AUTH" $BASE/api/categories)
n=$(echo "$cats" | grep -o '"id"' | wc -l | tr -d ' ')
[ "$n" -ge 10 ] && check "categories-seeded($n)" ok ok || check "categories-seeded" ok "$n"

# 5. accounts seeded
accs=$(curl -s -H "$AUTH" $BASE/api/accounts)
n=$(echo "$accs" | grep -o '"id"' | wc -l | tr -d ' ')
[ "$n" -ge 4 ] && check "accounts-seeded($n)" ok ok || check "accounts-seeded" ok "$n"

# 6. create expense tx
today=$(date +%Y-%m-%d)
month=$(date +%Y-%m)
r=$(curl -s -X POST $BASE/api/tx -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"type\":1,\"amount\":2560,\"category_id\":1,\"account_id\":1,\"date\":\"$today\",\"note\":\"lunch\"}")
echo "$r" | grep -q '"id":1' && check "create-expense-tx" ok ok || check "create-expense-tx" ok "$r"

# 7. create income tx
r=$(curl -s -X POST $BASE/api/tx -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"type\":2,\"amount\":500000,\"category_id\":10,\"account_id\":4,\"date\":\"$today\",\"note\":\"salary\"}")
echo "$r" | grep -q '"id":2' && check "create-income-tx" ok ok || check "create-income-tx" ok "$r"

# 8. invalid amount -> 400
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/tx -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"type\":1,\"amount\":0,\"category_id\":1,\"account_id\":1,\"date\":\"$today\"}")
check "invalid-amount-400" 400 "$code"

# 9. update tx
code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT $BASE/api/tx/1 -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"type\":1,\"amount\":3000,\"category_id\":2,\"account_id\":2,\"date\":\"$today\",\"note\":\"bus\"}")
check "update-tx-200" 200 "$code"

# 10. list sums
list=$(curl -s -H "$AUTH" "$BASE/api/tx?month=$month")
echo "$list" | grep -q '"sum_income":500000' && check "list-sum-income" ok ok || check "list-sum-income" ok "$list"
echo "$list" | grep -q '"sum_expense":3000' && check "list-sum-expense" ok ok || check "list-sum-expense" ok "$list"

# 11. report
rep=$(curl -s -H "$AUTH" "$BASE/api/report?month=$month")
echo "$rep" | grep -q '"expense":3000' && check "report-expense" ok ok || check "report-expense" ok "$rep"
echo "$rep" | grep -q '"trend"' && check "report-trend" ok ok || check "report-trend" ok "missing"
echo "$rep" | grep -q '"net":497000' && check "report-net" ok ok || check "report-net" ok "$rep"

# 12. budget put + get
code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT $BASE/api/budgets -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"category_id\":1,\"month\":\"$month\",\"amount\":50000}")
check "put-budget-200" 200 "$code"
bud=$(curl -s -H "$AUTH" "$BASE/api/budgets?month=$month")
echo "$bud" | grep -q '"budget":50000' && check "budget-saved" ok ok || check "budget-saved" ok "$bud"

# 13. delete budget with 0
code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT $BASE/api/budgets -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"category_id\":1,\"month\":\"$month\",\"amount\":0}")
check "clear-budget-200" 200 "$code"

# 14. delete tx
code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE $BASE/api/tx/2 -H "$AUTH")
check "delete-tx-200" 200 "$code"
code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE $BASE/api/tx/2 -H "$AUTH")
check "delete-tx-again-404" 404 "$code"

# 15. category in use -> cannot delete
code=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE $BASE/api/categories/2 -H "$AUTH")
check "delete-used-category-400" 400 "$code"

# 16. change password + login with new
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/password -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"old_password":"admin123","new_password":"newpass456"}')
check "change-password-200" 200 "$code"
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST $BASE/api/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"newpass456"}')
check "login-new-pass-200" 200 "$code"

# 17. static pages served
code=$(curl -s -o /dev/null -w '%{http_code}' $BASE/)
check "index-served-200" 200 "$code"
code=$(curl -s -o /dev/null -w '%{http_code}' $BASE/login.html)
check "login-served-200" 200 "$code"
code=$(curl -s -o /dev/null -w '%{http_code}' $BASE/app.js)
check "appjs-served-200" 200 "$code"

# 18. CSV import (JSON rows): creates new category+account, normalizes date
imp=$(curl -s -X POST $BASE/api/import -H "$AUTH" -H 'Content-Type: application/json' -d '{"rows":[
  {"date":"2026/11/8","type":"支出","amount":"25.60","category":"外卖","account":"饭卡","note":"import-a"},
  {"date":"2026-11-08","type":"收入","amount":"100","category":"红包","account":"现金","note":"import-b"},
  {"date":"2026.11.08","type":"支出","amount":"¥9.90","category":"交通","account":"微信","note":"import-c"}
]}')
echo "$imp" | grep -q '"imported":3' && check "import-3-rows" ok ok || check "import-3-rows" ok "$imp"

# 19. re-import same rows -> all skipped (idempotent)
imp2=$(curl -s -X POST $BASE/api/import -H "$AUTH" -H 'Content-Type: application/json' -d '{"rows":[
  {"date":"2026-11-08","type":"支出","amount":"25.60","category":"外卖","account":"饭卡","note":"import-a"},
  {"date":"2026-11-08","type":"收入","amount":"100","category":"红包","account":"现金","note":"import-b"},
  {"date":"2026-11-08","type":"支出","amount":"9.90","category":"交通","account":"微信","note":"import-c"}
]}')
echo "$imp2" | grep -q '"imported":0,"skipped":3' && check "import-idempotent" ok ok || check "import-idempotent" ok "$imp2"

# 20. import with a bad row -> whole batch rolled back
code_bad=$(curl -s -X POST $BASE/api/import -H "$AUTH" -H 'Content-Type: application/json' -d '{"rows":[
  {"date":"2026-11-07","type":"支出","amount":"10","category":"餐饮","account":"现金","note":"rollback-good"},
  {"date":"bad-date","type":"支出","amount":"1","category":"餐饮","account":"现金","note":"rollback-bad"}
]}')
echo "$code_bad" | grep -q '"failed":1' && check "import-rollback" ok ok || check "import-rollback" ok "$code_bad"
list_rollback=$(curl -s -H "$AUTH" "$BASE/api/tx?q=rollback-good")
echo "$list_rollback" | grep -q '"total":0' && check "import-rollback-no-residue" ok ok || check "import-rollback-no-residue" ok "$list_rollback"

echo "-----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" = "0" ]
