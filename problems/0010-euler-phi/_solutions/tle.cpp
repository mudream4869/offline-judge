// O(n log n): count i with gcd(i, n) = 1.
#include <bits/stdc++.h>
using namespace std;

int main() {
  long long n, c = 0;
  cin >> n;
  for (long long i = 1; i <= n; i++) c += gcd(i, n) == 1;
  cout << c << '\n';
}
