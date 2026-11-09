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

echo "-----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" = "0" ]
