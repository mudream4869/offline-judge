#include <bits/stdc++.h>
using namespace std;

int main() {
  long long lo = 1, hi, mid;
  cin >> hi;
  string r;
  while (true) {
    mid = (lo + hi) / 2;
    cout << "? " << mid << endl;
    cin >> r;
    if (r == "=") break;
    if (r == "<") hi = mid - 1;
    else lo = mid + 1;
  }
  cout << "! " << mid << endl;
}
