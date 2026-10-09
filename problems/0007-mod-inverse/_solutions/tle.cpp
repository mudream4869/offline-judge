// O(p): x = 1, 2, ... keeping a * x mod p.
#include <bits/stdc++.h>
using namespace std;

int main() {
  long long p, a;
  cin >> p >> a;
  long long x = 1, r = a;
  while (r != 1) {
    x++;
    r += a;
    if (r >= p) r -= p;
  }
  cout << x << '\n';
}
