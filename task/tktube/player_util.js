// Copyright 2026 The Cocomhub Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

var flashvars = {};

function bX(a) {
	return ""; // Dummy implementation to prevent crash if missing
}

function step1(a, b, c, d, e) {
    for (var f in a)
        if (0 == a[f].indexOf(b)) {
            var g = a[f].substring(b.length).split(b[b.length - 1]);

            var h = g[6].substring(0, 2 * parseInt(d)),
                i = e ? e(a, c, d) : "";

            if (i && h) {
                for (var j = h, k = h.length - 1; k >= 0; k--) {
                    for (var l = k, m = k; m < i.length; m++)
                        l += parseInt(i[m]);
                    for (; l >= h.length;)
                        l -= h.length;
                    for (var n = "", o = 0; o < h.length; o++)
                        n += o == k ? h[l] : o == l ? h[k] : h[o];
                    h = n
                }
                g[6] = g[6].replace(j, h),
                    g.splice(0, 1),
                    a[f] = g.join(b[b.length - 1])
            }
        }
}

function step2(a, b, c) {
    var e, g, h, i, j, k, l, m, n, d = "",
        f = "",
        o = parseInt;
    for (e in a)
        if (e.indexOf(b) > 0 && a[e].length == o(c)) {
            d = a[e];
            break
        }
    if (d) {
        for (f = "",
            g = 1; g < d.length; g++)
            f += o(d[g]) ? o(d[g]) : 1;
        for (j = o(f.length / 2),
            k = o(f.substring(0, j + 1)),
            l = o(f.substring(j)),
            g = l - k,
            g < 0 && (g = -g),
            f = g,
            g = k - l,
            g < 0 && (g = -g),
            f += g,
            f *= 2,
            f = "" + f,
            i = o(c) / 2 + 2,
            m = "",
            g = 0; g < j + 1; g++)
            for (h = 1; h <= 4; h++)
                n = o(d[g + h]) + o(f[g]),
                n >= i && (n -= i),
                m += n;
        return m
    }
    return d
}

function b$() {
    return (new Date).getTime()
}

function cm() {
    var a = Array.prototype.slice.call(arguments);
    return a.join(bX(2))
}

function get_list(a) {
    var z = [];
    if (!!a) {
        var b, c = 'video_url',
            d, e, f, g = !1,
            h = parseInt(a['default_slot']) || 1,
            i, j;
        f = '720p';
        a['skip_selected_format'] == 'true' && (f = null);
        a['rnd'] = b$();
        for (b = 0; b <= 7; b++)
            b > 0 && (c = 'video_alt_url',
                b > 1 && (c += b)),
            a[c] && (d = a[c],
                e = [
                    d,
                    d.toLowerCase().indexOf('.flv') > 0 ? 'video/flash' : 'video/mp4',
                    a[c + '_text'] || '',
                    a[c + '_redirect'] || 0,
                    (a[c + '_4k'] ? 2 : a[c + '_hd'] ? 1 : 0) || 0,
                    f ? f == a[c + '_text'] : !1,
                    a['preview_url'],
                ],
                i && (e[0] = cm(d, d.indexOf('?') >= 0 ? '&' : '?', 'rnd=', a['rnd'])),
                z.push(e),
                e[5] && (g = !0,
                    e[3] && (e[5] = !1,
                        g = !1)));
    }
    return z;
}

function main() {
    step1(flashvars, 'function/', 'code', "16px", step2);
    list = get_list(flashvars);
    return list[list.length - 1]
}
