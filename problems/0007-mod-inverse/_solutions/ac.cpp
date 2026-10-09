#include <bits/stdc++.h>
using namespace std;

int main() {
  long long p, a, r = 1;
  cin >> p >> a;
  for (long long e = p - 2; e; e >>= 1, a = a * a % p)
    if (e & 1) r = r * a % p;
  cout << r << '\n';
}
