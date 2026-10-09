// O(nk) Pascal's triangle.
#include <bits/stdc++.h>
using namespace std;
const int M = 1e9 + 7;

int main() {
  int n, k;
  cin >> n >> k;
  vector<int> c(k + 1);
  c[0] = 1;
  for (int i = 1; i <= n; i++)
    for (int j = min(i, k); j > 0; j--) c[j] = (c[j] + c[j - 1]) % M;
  cout << c[k] << '\n';
}
