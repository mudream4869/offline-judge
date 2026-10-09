// O(NQ) scan.
#include <bits/stdc++.h>
using namespace std;

int main() {
  ios::sync_with_stdio(false);
  cin.tie(nullptr);
  int n, q;
  cin >> n >> q;
  vector<long long> a(n);
  for (auto &x : a) cin >> x;
  string op;
  long long x, y;
  while (q--) {
    cin >> op >> x >> y;
    if (op == "set") a[x - 1] = y;
    else cout << *min_element(a.begin() + x - 1, a.begin() + y) << '\n';
  }
}
