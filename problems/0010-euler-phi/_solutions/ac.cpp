// Trial division up to sqrt(n).
#include <bits/stdc++.h>
using namespace std;

int main() {
  long long n;
  cin >> n;
  long long r = n;
  for (long long p = 2; p * p <= n; p++) {
    if (n % p) continue;
    while (n % p == 0) n /= p;
    r -= r / p;
  }
  if (n > 1) r -= r / n;
  cout << r << '\n';
}
