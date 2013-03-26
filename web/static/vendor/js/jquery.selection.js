/*!
 * jQuery.selection - jQuery Plugin
 *
 * Copyright (c) 2010-2012 IWASAKI Koji (@madapaja).
 * http://blog.madapaja.net/
 * Under The MIT License
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
 * LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
 * OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
 * WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */
(function($, win, doc) {
    /**
     * 要素の文字列選択状態を取得します
     *
     * @param   {Element}   element         対象要素
     * @return  {Object}    return
     * @return  {String}    return.text     選択されている文字列
     * @return  {Integer}   return.start    選択開始位置
     * @return  {Integer}   return.end      選択終了位置
     */
    var _getCaretInfo = function(element){
        var res = {
            text: '',
            start: 0,
            end: 0
        };

        if (!element.value) {
            /* 値がない、もしくは空文字列 */
            return res;
        }

        try {
            if (win.getSelection) {
                /* IE 以外 */
                res.start = element.selectionStart;
                res.end = element.selectionEnd;
                res.text = element.value.slice(res.start, res.end);
            } else if (doc.selection) {
                /* for IE */
                element.focus();

                var range = doc.selection.createRange(),
                    range2 = doc.body.createTextRange(),
                    tmpLength;

                res.text = range.text;

                try {
                    range2.moveToElementText(element);
                    range2.setEndPoint('StartToStart', range);
                } catch (e) {
                    range2 = element.createTextRange();
                    range2.setEndPoint('StartToStart', range);
                }

                res.start = element.value.length - range2.text.length;
                res.end = res.start + range.text.length;
            }
        } catch (e) {
            /* あきらめる */
        }

        return res;
    };

    /**
     * 要素に対するキャレット操作
     * @type {Object}
     */
    var _CaretOperation = {
        /**
         * 要素のキャレット位置を取得します
         *
         * @param   {Element}   element         対象要素
         * @return  {Object}    return
         * @return  {Integer}   return.start    選択開始位置
         * @return  {Integer}   return.end      選択終了位置
         */
        getPos: function(element) {
            var tmp = _getCaretInfo(element);
            return {start: tmp.start, end: tmp.end};
        },

        /**
         * 要素のキャレット位置を設定します
         *
         * @param   {Element}   element         対象要素
         * @param   {Object}    toRange         設定するキャレット位置
         * @param   {Integer}   toRange.start   選択開始位置
         * @param   {Integer}   toRange.end     選択終了位置
         * @param   {String}    caret           キャレットモード "keep" | "start" | "end" のいずれか
         */
        setPos: function(element, toRange, caret) {
            caret = this._caretMode(caret);

            if (caret == 'start') {
                toRange.end = toRange.start;
            } else if (caret == 'end') {
                toRange.start = toRange.end;
            }

            element.focus();
            try {
                if (element.createTextRange) {
                    var range = element.createTextRange();

                    if (win.navigator.userAgent.toLowerCase().indexOf("msie") >= 0) {
                        toRange.start = element.value.substr(0, toRange.start).replace(/\r/g, '').length;
                        toRange.end = element.value.substr(0, toRange.end).replace(/\r/g, '').length;
                    }

                    range.collapse(true);
                    range.moveStart('character', toRange.start);
                    range.moveEnd('character', toRange.end - toRange.start);

                    range.select();
                } else if (element.setSelectionRange) {
                    element.setSelectionRange(toRange.start, toRange.end);
                }
            } catch (e) {
                /* あきらめる */
            }
        },

        /**
         * 要素内の選択文字列を取得します
         *
         * @param   {Element}   element         対象要素
         * @return  {String}    return          選択文字列
         */
        getText: function(element) {
            return _getCaretInfo(element).text;
        },

        /**
         * キャレットモードを選択します
         *
         * @param   {String}    caret           キャレットモード
         * @return  {String}    return          "keep" | "start" | "end" のいずれか
         */
        _caretMode: function(caret) {
            caret = caret || "keep";
            if (caret == false) {
                caret = 'end';
            }

            switch (caret) {
                case 'keep':
                case 'start':
                case 'end':
                    break;

                default:
                    caret = 'keep';
            }

            return caret;
        },

        /**
         * 選択文字列を置き換えます
         *
         * @param   {Element}   element         対象要素
         * @param   {String}    text            置き換える文字列
         * @param   {String}    caret           キャレットモード "keep" | "start" | "end" のいずれか
         */
        replace: function(element, text, caret) {
            var tmp = _getCaretInfo(element),
                orig = element.value,
                pos = $(element).scrollTop(),
                range = {start: tmp.start, end: tmp.start + text.length};

            element.value = orig.substr(0, tmp.start) + text + orig.substr(tmp.end);

            $(element).scrollTop(pos);
            this.setPos(element, range, caret);
        },

        /**
         * 文字列を選択文字列の前に挿入します
         *
         * @param   {Element}   element         対象要素
         * @param   {String}    text            挿入文字列
         * @param   {String}    caret           キャレットモード "keep" | "start" | "end" のいずれか
         */
        insertBefore: function(element, text, caret) {
            var tmp = _getCaretInfo(element),
                orig = element.value,
                pos = $(element).scrollTop(),
                range = {start: tmp.start + text.length, end: tmp.end + text.length};

            element.value = orig.substr(0, tmp.start) + text + orig.substr(tmp.start);

            $(element).scrollTop(pos);
            this.setPos(element, range, caret);
        },

        /**
         * 文字列を選択文字列の後に挿入します
         *
         * @param   {Element}   element         対象要素
         * @param   {String}    text            挿入文字列
         * @param   {String}    caret           キャレットモード "keep" | "start" | "end" のいずれか
         */
        insertAfter: function(element, text, caret) {
            var tmp = _getCaretInfo(element),
                orig = element.value,
                pos = $(element).scrollTop(),
                range = {start: tmp.start, end: tmp.end};

            element.value = orig.substr(0, tmp.end) + text + orig.substr(tmp.end);

            $(element).scrollTop(pos);
            this.setPos(element, range, caret);
        }
    };

    /* jQuery.selection を追加 */
    $.extend({
        /**
         * ウィンドウの選択されている文字列を取得
         *
         * @param   {String}    mode            選択モード "text" | "html" のいずれか
         * @return  {String}    return
         */
        selection: function(mode) {
            var getText = ((mode || 'text').toLowerCase() == 'text');

            try {
                if (win.getSelection) {
                    if (getText) {
                        // get text
                        return win.getSelection().toString();
                    } else {
                        // get html
                        var sel = win.getSelection(), range;

                        if (sel.getRangeAt) {
                            range = sel.getRangeAt(0);
                        } else {
                            range = doc.createRange();
                            range.setStart(sel.anchorNode, sel.anchorOffset);
                            range.setEnd(sel.focusNode, sel.focusOffset);
                        }

                        return $('<div></div>').append(range.cloneContents()).html();
                    }
                } else if (doc.selection) {
                    if (getText) {
                        // get text
                        return doc.selection.createRange().text;
                    } else {
                        // get html
                        return doc.selection.createRange().htmlText;
                    }
                }
            } catch (e) {
                /* あきらめる */
            }

            return '';
        }
    });

    /* selection を追加 */
    $.fn.extend({
        selection: function(mode, opts) {
            opts = opts || {};

            switch (mode) {
                /**
                 * selection('getPos')
                 * キャレット位置を取得します
                 *
                 * @return  {Object}    return
                 * @return  {Integer}   return.start    選択開始位置
                 * @return  {Integer}   return.end      選択終了位置
                 */
                case 'getPos':
                    return _CaretOperation.getPos(this[0]);
                    break;

                /**
                 * selection('setPos', opts)
                 * キャレット位置を設定します
                 *
                 * @param   {Integer}   opts.start      選択開始位置
                 * @param   {Integer}   opts.end        選択終了位置
                 */
                case 'setPos':
                    return this.each(function() {
                        _CaretOperation.setPos(this, opts);
                    });
                    break;

                /**
                 * selection('replace', opts)
                 * 選択文字列を置き換えます
                 *
                 * @param   {String}    opts.text            置き換える文字列
                 * @param   {String}    opts.caret           キャレットモード "keep" | "start" | "end" のいずれか
                 */
                case 'replace':
                    return this.each(function() {
                        _CaretOperation.replace(this, opts.text, opts.caret);
                    });
                    break;

                /**
                 * selection('insert', opts)
                 * 選択文字列の前、もしくは後に文字列を挿入えます
                 *
                 * @param   {String}    opts.text            挿入文字列
                 * @param   {String}    opts.caret           キャレットモード "keep" | "start" | "end" のいずれか
                 * @param   {String}    opts.mode            挿入モード "before" | "after" のいずれか
                 */
                case 'insert':
                    return this.each(function() {
                        if (opts.mode == 'before') {
                            _CaretOperation.insertBefore(this, opts.text, opts.caret);
                        } else {
                            _CaretOperation.insertAfter(this, opts.text, opts.caret);
                        }
                    });

                    break;

                /**
                 * selection('get')
                 * 選択されている文字列を取得
                 *
                 * @return  {String}    return
                 */
                case 'get':
                default:
                    return _CaretOperation.getText(this[0]);
                    break;
            }

            return this;
        }
    });
})(jQuery, window, window.document);
