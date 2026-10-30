package main

import (
"fmt"
"time"

"github.com/golang-jwt/jwt/v5"
)

func main() {
claims := jwt.MapClaims{
"account_id": "69c813b937074cc36e9fe428",
"tenant_id":  "69c813f937074cc36e9fe42b",
"email":      "maria@email.com",
"role":       "admin",
"exp":        time.Now().Add(time.Hour).Unix(),
}

token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
ss, _ := token.SignedString([]byte("changeme_in_production"))
fmt.Println(ss)
}
