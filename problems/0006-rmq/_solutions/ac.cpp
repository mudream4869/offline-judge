// Iterative segment tree.
#include <bits/stdc++.h>
using namespace std;

int main() {
  ios::sync_with_stdio(false);
  cin.tie(nullptr);
  int n, q;
  cin >> n >> q;
  int size = 1;
  while (size < n) size *= 2;
  vector<long long> t(2 * size, LLONG_MAX);
  for (int i = 0; i < n; i++) cin >> t[size + i];
  for (int i = size - 1; i > 0; i--) t[i] = min(t[2 * i], t[2 * i + 1]);
  string op;
  long long a, b;
  while (q--) {
    cin >> op >> a >> b;
    if (op == "set") {
      int i = a - 1 + size;
      t[i] = b;
      for (i >>= 1; i; i >>= 1) t[i] = min(t[2 * i], t[2 * i + 1]);
    } else {
      long long res = LLONG_MAX;
      for (int l = a - 1 + size, r = b + size; l < r; l >>= 1, r >>= 1) {
        if (l & 1) res = min(res, t[l++]);
        if (r & 1) res = min(res, t[--r]);
      }
      cout << res << '\n';
    }
  }
}
