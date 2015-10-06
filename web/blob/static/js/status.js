/* globals PATH_PREFIX, $ */
$(function () {
    'use strict';

    $('#reload_conf_btn').on('click', function () {
        var elBtn;
        $(this).promise().then(function (btn) {
            elBtn = btn;
            elBtn.text('Waiting...').prop('disabled', true);
            return $.post(PATH_PREFIX + '/-/reload');
        }).then(function () {
            elBtn.text('Triggered Reload!')
                .prop('disabled', false)
                .addClass('btn-success');
        }, function() {
            elBtn.text('Triggered Reload Failed!')
                .prop('disabled', false)
                .addClass('btn-danger');
        }).always(function () {
            setTimeout(function () {
                elBtn.removeClass('btn-danger btn-success')
                    .text('Reload Configuration');
            }, 1400);
        });
    });
});
