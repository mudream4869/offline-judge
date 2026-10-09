#include <bits/stdc++.h>
using namespace std;

long long max_subarray(const vector<int>& a) {
    long long best = a[0];
    for (size_t i = 0; i < a.size(); i++) {
        long long s = 0;
        for (size_t j = i; j < a.size(); j++) {
            s += a[j];
            best = max(best, s);
        }
    }
    return best;
}
