#include <bits/stdc++.h>
using namespace std;

long long max_subarray(const vector<int>& a) {
    long long best = a[0], cur = a[0];
    for (size_t i = 1; i < a.size(); i++) {
        cur = max<long long>(a[i], cur + a[i]);
        best = max(best, cur);
    }
    return best;
}
