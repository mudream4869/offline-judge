// O(k) products, one inverse.
#include <bits/stdc++.h>
using namespace std;
const long long M = 1e9 + 7;

long long power(long long a, long long e) {
  long long r = 1;
  for (; e; e >>= 1, a = a * a % M)
    if (e & 1) r = r * a % M;
  return r;
}

int main() {
  long long n, k;
  cin >> n >> k;
  k = min(k, n - k);
  long long num = 1, den = 1;
  for (long long i = 0; i < k; i++) {
    num = num * ((n - i) % M) % M;
    den = den * (i + 1) % M;
  }
  cout << num * power(den, M - 2) % M << '\n';
}
